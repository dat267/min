// Package cli provides a zero-dependency CLI parser using struct tags.
//
// Define command structs with help, short, default, arg, cmd, required,
// choices, and hidden tags, then call New().Parse().
//
// Resolution order: CLI args > env vars > config file > struct defaults.
//
// Subcommands, positional args, combined short flags, and interactive
// prompting for required flags are supported out of the box.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

// Tag holds the parsed struct tag values for a single struct field.
// Populated by parseTag from the field's reflect.StructField.
type Tag struct {
	Help    string   // help text shown in --help output
	Short   string   // single-character short flag alias (e.g. "v" for -v)
	Default string   // default value string from struct tag
	Arg     bool     // field is a positional argument (arg:"")
	Cmd     bool     // field is a subcommand group (cmd:"")
	Req     bool     // flag is required (required:"")
	Hidden  bool     // suppress in help output (hidden:"")
	Choices []string // allowed values from choices:"a,b,c" tag
}

func parseTag(ft reflect.StructField) Tag {
	t := Tag{}
	has := func(key string) bool { _, ok := ft.Tag.Lookup(key); return ok }
	t.Help = ft.Tag.Get("help")
	t.Short = ft.Tag.Get("short")
	t.Default = ft.Tag.Get("default")
	t.Arg = has("arg")
	t.Cmd = has("cmd")
	t.Req = has("required")
	t.Hidden = has("hidden")
	if c := ft.Tag.Get("choices"); c != "" {
		raw := strings.Split(c, ",")
		t.Choices = make([]string, len(raw))
		for i, s := range raw {
			t.Choices[i] = strings.TrimSpace(s)
		}
	}
	return t
}

type flag struct {
	name     string
	tag      Tag
	val      reflect.Value
	env      string
	explicit bool
}

type cmd struct {
	name   string
	help   string
	hidden bool
	val    reflect.Value
	flags  []*flag
	args   []*flag
	subs   []*cmd
	parent *cmd
}

// App is the top-level CLI application. Created via New().
type App struct {
	name       string
	desc       string
	root       reflect.Value
	cfg        string
	pre        string
	prompt     bool
	binds      map[reflect.Type]reflect.Value
	ver        string
	cfgLoaded  bool
	cfgFlat    map[string]any
	defCmd     string
	hasYes     bool
	cfgField   string
}

// ConfigPath returns the active config file path.
func (a *App) ConfigPath() string { return a.cfg }

// WithName sets the app name shown in usage text.
func WithName(s string) Option { return func(a *App) { a.name = s } }

// WithDesc sets the app description shown below the usage line.
func WithDesc(s string) Option { return func(a *App) { a.desc = s } }

// WithCfg sets the default config file path.
func WithCfg(s string) Option { return func(a *App) { a.cfg = s } }

// WithEnv sets the env var prefix for auto-derived flag env vars.
func WithEnv(s string) Option { return func(a *App) { a.pre = s } }

// WithPrompt enables interactive prompting for required flags.
func WithPrompt() Option { return func(a *App) { a.prompt = true } }

// WithVersion enables the --version flag.
func WithVersion(s string) Option { return func(a *App) { a.ver = s } }

// WithDefaultCmd sets a default subcommand name.
func WithDefaultCmd(s string) Option { return func(a *App) { a.defCmd = s } }

// WithConfigField sets the struct field name for config path injection.
func WithConfigField(s string) Option { return func(a *App) { a.cfgField = s } }

// Option configures an App. Used with New().
type Option func(*App)

// New creates an App from a root struct pointer. Panics if root is nil
// or not a pointer to a struct.
func New(root any, opts ...Option) *App {
	rv := reflect.ValueOf(root)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		panic("cli: New() requires a non-nil pointer to a struct")
	}
	if rv.Elem().Kind() != reflect.Struct {
		panic("cli: New() requires a non-nil pointer to a struct")
	}
	a := &App{root: rv.Elem(), binds: map[reflect.Type]reflect.Value{}}
	for _, o := range opts {
		o(a)
	}
	return a
}

// Bind registers a value for dependency injection into Run() methods.
// The value's type is used as the lookup key when resolving Run parameters.
func (a *App) Bind(v any) { a.binds[reflect.TypeOf(v)] = reflect.ValueOf(v) }

func kebab(s string) string {
	var b strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				prev := rune(s[i-1])
				switch {
				case prev >= 'a' && prev <= 'z':
					b.WriteRune('-')
				case prev >= 'A' && prev <= 'Z' && i+1 < len(s) && rune(s[i+1]) >= 'a' && rune(s[i+1]) <= 'z':
					b.WriteRune('-')
				case prev >= '0' && prev <= '9':
					b.WriteRune('-')
				}
			}
			b.WriteRune(r + 32)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (a *App) build(v reflect.Value, parent *cmd) *cmd {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}
	t := v.Type()
	c := &cmd{name: t.Name(), val: v, parent: parent}
	for i, fieldCount := 0, t.NumField(); i < fieldCount; i++ {
		ft := t.Field(i)
		fv := v.Field(i)
		if !fv.CanSet() || ft.Tag.Get("json") == "-" {
			continue
		}
		tg := parseTag(ft)
		fieldName := kebab(ft.Name)
		switch {
		case tg.Cmd:
			ch := a.build(fv, c)
			if ch == nil {
				continue
			}
			ch.name = fieldName
			ch.help = tg.Help
			ch.hidden = tg.Hidden
			c.subs = append(c.subs, ch)
		case tg.Arg:
			c.args = append(c.args, &flag{name: fieldName, tag: tg, val: fv})
		default:
			fl := &flag{name: fieldName, tag: tg, val: fv}
			if a.pre != "" {
				fl.env = a.pre + strings.ToUpper(strings.ReplaceAll(fieldName, "-", "_"))
			}
			c.flags = append(c.flags, fl)
		}
	}
	return c
}

var durationType = reflect.TypeFor[time.Duration]()

func set(v reflect.Value, s string) bool {
	if v.Type() == durationType {
		d, err := time.ParseDuration(s)
		if err == nil {
			v.SetInt(int64(d))
			return true
		}
		return false
	}
	switch v.Kind() {
	case reflect.String:
		v.SetString(s)
		return true
	case reflect.Bool:
		v.SetBool(s == "true" || s == "1" || s == "")
		return true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n := int64(0)
		if _, err := fmt.Sscanf(s, "%d", &n); err == nil {
			v.SetInt(n)
			return true
		}
		return false
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n := uint64(0)
		if _, err := fmt.Sscanf(s, "%d", &n); err == nil {
			v.SetUint(n)
			return true
		}
		return false
	case reflect.Float32, reflect.Float64:
		f := float64(0)
		if _, err := fmt.Sscanf(s, "%f", &f); err == nil {
			v.SetFloat(f)
			return true
		}
		return false
	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.String {
			v.Set(reflect.Append(v, reflect.ValueOf(s)))
			return true
		}
		return false
	}
	return false
}

func (a *App) resolve(fl *flag) {
	if fl.explicit {
		return
	}
	if fl.env != "" {
		if v, ok := os.LookupEnv(fl.env); ok {
			if set(fl.val, v) && !validate(fl) {
				fmt.Fprintf(os.Stderr, "warning: value %q not in choices %v for env %s, falling back to default\n", v, fl.tag.Choices, fl.env)
				a.applyDefault(fl)
				return
			}
			return
		}
	}
	if a.cfg != "" {
		a.loadCfg()
		if v, ok := a.cfgFlat[fl.name]; ok && v != nil {
			vs := fmt.Sprintf("%v", v)
			if set(fl.val, vs) && !validate(fl) {
				fmt.Fprintf(os.Stderr, "warning: value %v not in choices %v for config key %s, falling back to default\n", v, fl.tag.Choices, fl.name)
				a.applyDefault(fl)
				return
			}
			return
		}
	}
	if fl.tag.Default != "" && fl.val.IsZero() {
		set(fl.val, fl.tag.Default)
		if !validate(fl) {
			fmt.Fprintf(os.Stderr, "warning: default %q not in choices %v for flag --%s\n", fl.tag.Default, fl.tag.Choices, fl.name)
		}
	}
}

func (a *App) applyDefault(fl *flag) {
	if fl.tag.Default == "" {
		return
	}
	set(fl.val, fl.tag.Default)
	if !validate(fl) {
		fmt.Fprintf(os.Stderr, "warning: default %q not in choices %v for flag --%s\n", fl.tag.Default, fl.tag.Choices, fl.name)
	}
}

func validate(fl *flag) bool {
	if len(fl.tag.Choices) == 0 || fl.val.Kind() == reflect.Slice {
		return true
	}
	for _, c := range fl.tag.Choices {
		switch fl.val.Kind() {
		case reflect.String:
			if fl.val.String() == c {
				return true
			}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			n := int64(0)
			if _, err := fmt.Sscanf(c, "%d", &n); err == nil && fl.val.Int() == n {
				return true
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			n := uint64(0)
			if _, err := fmt.Sscanf(c, "%d", &n); err == nil && fl.val.Uint() == n {
				return true
			}
		case reflect.Float32, reflect.Float64:
			f := float64(0)
			if _, err := fmt.Sscanf(c, "%f", &f); err == nil && fl.val.Float() == f {
				return true
			}
		}
	}
	return false
}

func (a *App) loadCfg() {
	if a.cfgLoaded {
		return
	}
	a.cfgLoaded = true
	data, err := os.ReadFile(filepath.Clean(a.cfg))
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "warning: config file %s: %v\n", filepath.Clean(a.cfg), err)
		}
		return
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		fmt.Fprintf(os.Stderr, "warning: config file %s: %v\n", filepath.Clean(a.cfg), err)
		return
	}
	flat := map[string]any{}
	var fn func(m map[string]any, p string)
	fn = func(m map[string]any, p string) {
		for k, v := range m {
			key := k
			if p != "" {
				key = p + "-" + k
			}
			if sub, ok := v.(map[string]any); ok {
				fn(sub, key)
			} else if v != nil {
				flat[key] = v
			}
		}
	}
	fn(raw, "")
	a.cfgFlat = flat
}

func (a *App) allFlags(cur *cmd) []*flag {
	var fl []*flag
	seen := map[string]bool{}
	for c := cur; c != nil; c = c.parent {
		for _, f := range c.flags {
			if !seen[f.name] {
				seen[f.name] = true
				fl = append(fl, f)
			}
		}
	}
	return fl
}

func builtinNoArg(name string) bool  { return name == "h" || name == "help" || name == "y" || name == "yes" }
func builtinHasArg(name string) bool  { return name == "config-file" }

func (a *App) flagConsumesNext(arg string, args []string, i int, deep []*flag) bool {
	if !strings.HasPrefix(arg, "-") || strings.Contains(arg, "=") {
		return false
	}
	if i+1 >= len(args) || args[i+1] == "--" {
		return false
	}
	name := strings.TrimLeft(arg, "-")
	if builtinNoArg(name) {
		return false
	}
	if builtinHasArg(name) {
		return true
	}
	if len(arg) > 1 && arg[1] != '-' && len(arg) > 2 {
		return false
	}
	f := a.flagByName(name, deep)
	return f != nil && f.val.Kind() != reflect.Bool
}

func (a *App) allFlagsDeep(root *cmd) []*flag {
	var fl []*flag
	seen := map[string]bool{}
	var walk func(*cmd)
	walk = func(c *cmd) {
		for _, f := range c.flags {
			if !seen[f.name] {
				seen[f.name] = true
				fl = append(fl, f)
			}
		}
		for _, s := range c.subs {
			walk(s)
		}
	}
	walk(root)
	return fl
}

func (a *App) setFlag(f *flag, val string, args []string, i *int) {
	if f.val.Kind() == reflect.Bool {
		set(f.val, "true")
		if val == "false" || val == "0" {
			set(f.val, "false")
		}
		f.explicit = true
	} else {
		if val == "" && *i+1 < len(args) && args[*i+1] != "--" {
			*i++
			val = args[*i]
		}
		set(f.val, val)
		f.explicit = validate(f)
		if !f.explicit && f.tag.Default != "" {
			set(f.val, f.tag.Default)
			f.explicit = true
		}
	}
}

func (a *App) flagByName(name string, flags []*flag) *flag {
	for _, f := range flags {
		if f.name == name || f.tag.Short == name {
			return f
		}
	}
	return nil
}

func (a *App) parseOne(args []string, i *int, flags []*flag, cur *cmd) error {
	arg := args[*i]
	if !strings.HasPrefix(arg, "-") {
		return nil
	}
	if len(arg) < 2 {
		a.help(cur)
		return fmt.Errorf("invalid flag %q", arg)
	}

	// Combined short flags: -abc (exactly one dash, multiple chars, not =value)
	if arg[0] == '-' && arg[1] != '-' && !strings.Contains(arg, "=") {
		for ci, ch := range arg[1:] {
			if ch == 'h' {
				a.help(cur)
				return errHelp
			}
			if ch == 'y' {
				a.hasYes = true
				if rest := arg[ci+2:]; rest == "false" || rest == "0" {
					a.hasYes = false
				}
				continue
			}
			rest := arg[ci+2:]
			f := a.flagByName(string(ch), flags)
			if f == nil {
				a.help(cur)
				return fmt.Errorf("unknown flag -%c", ch)
			}
			if f.val.Kind() == reflect.Bool {
				set(f.val, "true")
				f.explicit = true
				if rest == "false" || rest == "0" {
					set(f.val, "false")
					break
				}
			} else {
				if rest != "" {
					set(f.val, rest)
				} else if *i+1 < len(args) && args[*i+1] != "--" {
					*i++
					set(f.val, args[*i])
				}
				f.explicit = validate(f)
				if !f.explicit && f.tag.Default != "" {
					set(f.val, f.tag.Default)
					f.explicit = true
				}
				break
			}
		}
		return nil
	}

	// Long flag (--xxx) or single short flag (-x)
	name := strings.TrimLeft(arg, "-")
	val := ""
	if k, v, ok := strings.Cut(name, "="); ok {
		name, val = k, v
	}
	if name == "help" || name == "h" {
		a.help(cur)
		return errHelp
	}
	if name == "yes" || name == "y" {
		a.hasYes = true
		if val == "false" || val == "0" {
			a.hasYes = false
		}
		return nil
	}
	if name == "config-file" {
		if val != "" {
			a.cfg = val
		} else if *i+1 < len(args) && args[*i+1] != "--" {
			*i++
			a.cfg = args[*i]
		}
		return nil
	}
	if name == "version" && a.ver != "" {
		fmt.Println(a.ver)
		return errHelp
	}
	f := a.flagByName(name, flags)
	if f != nil {
		a.setFlag(f, val, args, i)
		return nil
	}
	a.help(cur)
	return fmt.Errorf("unknown flag %q", arg)
}

var errHelp = fmt.Errorf("help")

// Parse parses CLI args, resolves env/config/defaults, and dispatches to Run.
// Returns nil on success, help display, or --version. Returns error on
// unknown flags, missing commands, or Run() errors.
func (a *App) Parse(args []string) error {
	r := a.build(a.root, nil)
	deep := a.allFlagsDeep(r)

	// Find subcommand boundary: skip flags and their values
	cmdIdx := len(args)
	cmdIdxFound := false
loop:
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			cmdIdx = i
			cmdIdxFound = true
			break loop
		}
		if !strings.HasPrefix(arg, "-") {
			for _, s := range r.subs {
				if s.name == arg {
					cmdIdx = i
					cmdIdxFound = true
					break loop
				}
			}
			break
		}
		if a.flagConsumesNext(args[i], args, i, deep) {
			i++
		}
	}
	if !cmdIdxFound {
		cmdIdx = 0
	}

	// Process pre-subcommand flags on root
	for i := 0; i < cmdIdx; i++ {
		if err := a.parseOne(args, &i, a.allFlags(r), r); err != nil {
			if err == errHelp {
				return nil
			}
			return err
		}
	}

	// Navigate subcommand chain
	cur := r
	remain := args[cmdIdx:]
	for {
		found := false
		for i := 0; i < len(remain); i++ {
			arg := remain[i]
			if arg == "--" {
				break // stop routing at terminator
			}
			if a.flagConsumesNext(arg, remain, i, deep) {
				i++
			} else if !strings.HasPrefix(arg, "-") {
				for _, s := range cur.subs {
					if s.name == arg {
						cur = s
						remain = remain[i+1:]
						found = true
						break
					}
				}
				if found {
					break
				}
				break
			}
		}
		if !found {
			break
		}
	}

	// Process positional args and flags on the final command
	allFlags := a.allFlags(cur)
	pos := 0
	for i := 0; i < len(remain); i++ {
		arg := remain[i]
		if arg == "--" {
			for i++; i < len(remain); i++ {
				if pos < len(cur.args) {
					cur.args[pos].explicit = true
					set(cur.args[pos].val, remain[i])
					if !validate(cur.args[pos]) && cur.args[pos].tag.Default != "" {
						set(cur.args[pos].val, cur.args[pos].tag.Default)
					}
					if pos < len(cur.args)-1 || cur.args[pos].val.Kind() != reflect.Slice {
						pos++
					}
				}
			}
			break
		}
		if strings.HasPrefix(arg, "-") {
			if err := a.parseOne(remain, &i, allFlags, cur); err != nil {
				if err == errHelp {
					return nil
				}
				return err
			}
			continue
		}
		if pos < len(cur.args) {
			cur.args[pos].explicit = true
			set(cur.args[pos].val, arg)
			if !validate(cur.args[pos]) && cur.args[pos].tag.Default != "" {
				set(cur.args[pos].val, cur.args[pos].tag.Default)
			}
			if pos < len(cur.args)-1 || cur.args[pos].val.Kind() != reflect.Slice {
				pos++
			}
			continue
		}
		if len(cur.subs) > 0 {
			a.help(cur)
			return fmt.Errorf("unknown command %q", arg)
		}
		a.help(cur)
		return fmt.Errorf("unexpected argument %q", arg)
	}

	remainEmpty := len(remain) == 0

	// Default subcommand fallback
	if cur == r && len(r.subs) > 0 && a.defCmd != "" {
		for _, s := range r.subs {
			if s.name == a.defCmd {
				cur = s
				break
			}
		}
	}

	// Resolve env > config > defaults for flags (all ancestors of current command)
	for _, f := range a.allFlags(cur) {
		a.resolve(f)
	}
	for _, f := range cur.args {
		if f.tag.Default != "" && !f.explicit {
			set(f.val, f.tag.Default)
		}
	}

	if err := a.checkRequired(cur); err != nil {
		return err
	}

	a.injectConfigPath()

	return a.dispatch(cur, remainEmpty, r)
}

func (a *App) checkRequired(cur *cmd) error {
	var missing []string
	for _, f := range cur.flags {
		if f.tag.Req && f.val.IsZero() {
			missing = append(missing, "--"+f.name)
		}
	}
	if len(missing) > 0 {
		if a.prompt && isInteractive() && !a.hasYes {
			a.promptFor(cur)
			missing = nil
			for _, f := range cur.flags {
				if f.tag.Req && f.val.IsZero() {
					missing = append(missing, "--"+f.name)
				}
			}
		}
		if len(missing) > 0 {
			a.help(cur)
			return fmt.Errorf("required: %s", strings.Join(missing, ", "))
		}
	}
	return nil
}

func (a *App) promptFor(cur *cmd) {
	for _, f := range cur.flags {
		if !f.tag.Req || !f.val.IsZero() {
			continue
		}
		switch f.val.Kind() {
		case reflect.String:
			fmt.Fprintf(os.Stderr, "%s (--%s): ", f.tag.Help, f.name)
			var v string
			if _, err := fmt.Scanln(&v); err != nil {
				return
			}
			set(f.val, strings.TrimSpace(v))
		case reflect.Bool:
			for {
				fmt.Fprintf(os.Stderr, "%s (--%s): ", f.tag.Help, f.name)
				var v string
				if _, err := fmt.Scanln(&v); err != nil {
					return
				}
				v = strings.TrimSpace(v)
				lower := strings.ToLower(v)
				if lower == "true" || lower == "1" || lower == "false" || lower == "0" {
					set(f.val, lower)
					break
				}
				fmt.Fprintf(os.Stderr, "  enter true/false\n")
			}
		default:
			for {
				fmt.Fprintf(os.Stderr, "%s (--%s): ", f.tag.Help, f.name)
				var v string
				if _, err := fmt.Scanln(&v); err != nil {
					return
				}
				v = strings.TrimSpace(v)
				if v == "" {
					fmt.Fprintf(os.Stderr, "  enter a value\n")
					continue
				}
				if set(f.val, v) {
					break
				}
				fmt.Fprintf(os.Stderr, "  invalid value\n")
			}
		}
	}
}

func (a *App) injectConfigPath() {
	name := a.cfgField
	if name == "" {
		name = "ConfigPath"
	}
	if cf := a.root.FieldByName(name); cf.IsValid() && cf.Kind() == reflect.String {
		cf.SetString(a.cfg)
		for t := range a.binds {
			if t.Kind() == reflect.String && t.Name() == name {
				a.binds[t] = cf
				break
			}
		}
	}
}

func (a *App) dispatch(cur *cmd, remainEmpty bool, r *cmd) error {
	v := cur.val
	if v.CanAddr() {
		v = v.Addr()
	}
	run := v.MethodByName("Run")
	if !run.IsValid() {
		if len(cur.subs) > 0 && remainEmpty {
			a.help(cur)
			if cur == r {
				return fmt.Errorf("no command specified")
			}
			return nil
		}
		return nil
	}
	rargs := []reflect.Value{}
	runT := run.Type()
	for i, n := 0, runT.NumIn(); i < n; i++ {
		t := runT.In(i)
		if bv, ok := a.binds[t]; ok {
			rargs = append(rargs, bv)
			continue
		}
		found := false
		for bt, bv := range a.binds {
			if bt.AssignableTo(t) {
				rargs = append(rargs, bv)
				found = true
				break
			}
		}
		if !found {
			if t.Kind() == reflect.Pointer {
				rargs = append(rargs, reflect.New(t.Elem()))
			} else {
				rargs = append(rargs, reflect.Zero(t))
			}
		}
	}
	rv := run.Call(rargs)
	if len(rv) > 0 && !rv[0].IsNil() {
		return rv[0].Interface().(error)
	}
	return nil
}

func (a *App) help(cur *cmd) {
	name := a.name
	if name == "" {
		name = filepath.Base(os.Args[0])
	}
	fmt.Print("Usage: ", name)
	if len(cur.subs) > 0 {
		fmt.Print(" <command>")
	}
	if len(cur.flags) > 0 {
		fmt.Print(" [flags]")
	}
	for _, f := range cur.args {
		n := strings.ToUpper(f.name)
		switch {
		case f.tag.Default != "":
			fmt.Printf(" [%s=%s]", n, f.tag.Default)
		case f.tag.Req:
			fmt.Printf(" <%s>", n)
		default:
			fmt.Printf(" [%s]", n)
		}
	}
	fmt.Println()
	if a.desc != "" {
		fmt.Println("\n" + a.desc)
	}

	if len(cur.subs) > 0 {
		fmt.Println("\nCommands:")
		max := 0
		for _, s := range cur.subs {
			if !s.hidden {
				if l := len(s.name); l > max {
					max = l
				}
			}
		}
		for _, s := range cur.subs {
			if s.hidden {
				continue
			}
			n := "  " + s.name
			if len(n) < max+6 {
				n += strings.Repeat(" ", max+6-len(n))
			} else {
				n += strings.Repeat(" ", 4)
			}
			fmt.Print(n)
			if s.help != "" {
				fmt.Print(s.help)
			}
			fmt.Println()
		}
	}

	fmt.Println("\nFlags:")
	fmt.Println("  -h, --help    Print help")
	if a.prompt {
		fmt.Println("  -y, --yes    Skip interactive prompts")
	}
	{
		configHelp := "      --config-file <PATH>    Path to config file"
		if a.pre != "" {
			configHelp += fmt.Sprintf(" [env: %sCONFIG_FILE]", a.pre)
		}
		fmt.Println(configHelp)
	}
	seen := map[string]bool{}
	for _, f := range a.allFlags(cur) {
		if seen[f.name] || f.tag.Hidden {
			continue
		}
		seen[f.name] = true
		b := "  "
		if f.tag.Short != "" {
			b += fmt.Sprintf("-%s, --%s", f.tag.Short, f.name)
		} else {
			b += fmt.Sprintf("    --%s", f.name)
		}
		if f.val.Kind() != reflect.Bool {
			b += fmt.Sprintf(" <%s>", strings.ToUpper(f.name))
		}
		pad := 30
		if len(b) < pad {
			b += strings.Repeat(" ", pad-len(b))
		} else {
			b += "  "
		}
		fmt.Print(b)
		if f.tag.Help != "" {
			fmt.Print(f.tag.Help)
		}
		if f.tag.Default != "" {
			fmt.Printf(" [default: %s]", f.tag.Default)
		}
		if len(f.tag.Choices) > 0 {
			fmt.Printf(" [choices: %s]", strings.Join(f.tag.Choices, ", "))
		}
		if f.env != "" {
			fmt.Printf(" [env: %s]", f.env)
		}
		fmt.Println()
	}
}

func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && (fi.Mode()&os.ModeCharDevice) != 0
}


