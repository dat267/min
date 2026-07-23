// Package cli provides a zero-dependency CLI parser using struct tags.
// Define command structs with help,short,default,arg,cmd,required tags,
// then call New().Parse().
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

type Tag struct {
	Help    string
	Short   string
	Default string
	Arg     bool
	Cmd     bool
	Req     bool
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
	return t
}

type flag struct {
	name string
	tag  Tag
	val  reflect.Value
	env  string
}

type cmd struct {
	name   string
	help   string
	val    reflect.Value
	flags  []*flag
	args   []*flag
	subs   []*cmd
	parent *cmd
}

type App struct {
	name      string
	desc      string
	root      reflect.Value
	cfg       string
	pre       string
	prompt    bool
	binds     map[reflect.Type]reflect.Value
	ver       string
	cfgLoaded bool
	cfgFlat   map[string]any
	defCmd    string
	hasYes    bool
}

func (a *App) ConfigPath() string { return a.cfg }

func WithName(s string) Option       { return func(a *App) { a.name = s } }
func WithDesc(s string) Option        { return func(a *App) { a.desc = s } }
func WithCfg(s string) Option         { return func(a *App) { a.cfg = s } }
func WithEnv(s string) Option         { return func(a *App) { a.pre = s } }
func WithPrompt() Option              { return func(a *App) { a.prompt = true } }
func WithVersion(s string) Option     { return func(a *App) { a.ver = s } }
func WithDefaultCmd(s string) Option  { return func(a *App) { a.defCmd = s } }

type Option func(*App)

func New(root any, opts ...Option) *App {
	a := &App{root: reflect.ValueOf(root).Elem(), binds: map[reflect.Type]reflect.Value{}}
	for _, o := range opts {
		o(a)
	}
	return a
}

func (a *App) Bind(v any) { a.binds[reflect.TypeOf(v)] = reflect.ValueOf(v) }

func kebab(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteRune('-')
		}
		if r >= 'A' && r <= 'Z' {
			b.WriteRune(r + 32)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (a *App) build(v reflect.Value, parent *cmd) *cmd {
	for v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	t := v.Type()
	c := &cmd{name: t.Name(), val: v, parent: parent}
	for i, n := 0, t.NumField(); i < n; i++ {
		ft := t.Field(i)
		fv := v.Field(i)
		if !fv.CanSet() || ft.Tag.Get("json") == "-" {
			continue
		}
		tg := parseTag(ft)
		n := kebab(ft.Name)
		if tg.Cmd {
			ch := a.build(fv, c)
			ch.name = n
			ch.help = tg.Help
			c.subs = append(c.subs, ch)
		} else if tg.Arg {
			c.args = append(c.args, &flag{name: n, tag: tg, val: fv})
		} else {
			fl := &flag{name: n, tag: tg, val: fv}
			if a.pre != "" {
				fl.env = a.pre + strings.ToUpper(strings.ReplaceAll(n, "-", "_"))
			}
			c.flags = append(c.flags, fl)
		}
	}
	return c
}

var durationType = reflect.TypeFor[time.Duration]()

func set(v reflect.Value, s string) {
	if v.Type() == durationType {
		d, err := time.ParseDuration(s)
		if err == nil {
			v.SetInt(int64(d))
		}
		return
	}
	switch v.Kind() {
	case reflect.String:
		v.SetString(s)
	case reflect.Bool:
		v.SetBool(s == "true" || s == "1" || s == "")
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n := int64(0)
		if _, err := fmt.Sscanf(s, "%d", &n); err == nil {
			v.SetInt(n)
		}
	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.String {
			v.Set(reflect.Append(v, reflect.ValueOf(s)))
		}
	}
}

func (a *App) resolve(fl *flag) {
	if !fl.val.IsZero() {
		return
	}
	if fl.env != "" {
		if v, ok := os.LookupEnv(fl.env); ok {
			set(fl.val, v)
			return
		}
	}
	if a.cfg != "" {
		a.loadCfg()
		if v, ok := a.cfgFlat[fl.name]; ok && v != nil {
			set(fl.val, fmt.Sprintf("%v", v))
			return
		}
	}
	if fl.tag.Default != "" && fl.val.IsZero() {
		set(fl.val, fl.tag.Default)
	}
}

func (a *App) loadCfg() {
	if a.cfgLoaded {
		return
	}
	a.cfgLoaded = true
	data, err := os.ReadFile(filepath.Clean(a.cfg))
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "warning: config file %s: %v\n", a.cfg, err)
		}
		return
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		fmt.Fprintf(os.Stderr, "warning: config file %s: %v\n", a.cfg, err)
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
	} else {
		if val == "" && *i+1 < len(args) && args[*i+1] != "--" {
			*i++
			val = args[*i]
		}
		set(f.val, val)
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
				break // non-bool consumes the rest
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

func (a *App) Parse(args []string) error {
	r := a.build(a.root, nil)
	deep := a.allFlagsDeep(r)

	// Find subcommand boundary: skip flags and their values
	cmdIdx := 0
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			cmdIdx = i
			goto foundSub
		}
		if !strings.HasPrefix(arg, "-") {
			for _, s := range r.subs {
				if s.name == arg {
					cmdIdx = i
					goto foundSub
				}
			}
			break
		}
		if a.flagConsumesNext(args[i], args, i, deep) {
			i++
		}
	}
foundSub:

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
					set(cur.args[pos].val, remain[i])
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
			set(cur.args[pos].val, arg)
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
				// Prepend the default subcommand name to remain so positional args still work
				break
			}
		}
	}

	// Resolve env > config > defaults for flags and args
	for _, f := range allFlags {
		a.resolve(f)
	}
	for _, f := range cur.args {
		if f.tag.Default != "" && f.val.IsZero() {
			set(f.val, f.tag.Default)
		}
	}

	// Required + prompt
	var missing []string
	for _, f := range cur.flags {
		if f.tag.Req && f.val.IsZero() {
			missing = append(missing, "--"+f.name)
		}
	}
	if len(missing) > 0 {
		if a.prompt && isInteractive() && !a.hasYes {
			for _, f := range cur.flags {
				if f.tag.Req && f.val.IsZero() {
					fmt.Fprintf(os.Stderr, "%s (--%s): ", f.tag.Help, f.name)
					var v string
					fmt.Scanln(&v)
					if s := strings.TrimSpace(v); s != "" {
						set(f.val, s)
					}
				}
			}
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

	if cf := a.root.FieldByName("ConfigPath"); cf.IsValid() && cf.Kind() == reflect.String {
		cf.SetString(a.cfg)
		// Also update the binding so Run() injection gets the live path
		for t := range a.binds {
			if t.Kind() == reflect.String && t.Name() == "ConfigPath" {
				a.binds[t] = cf
			}
		}
	}

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
	fmt.Print("Usage: ", a.name)
	if len(cur.subs) > 0 {
		fmt.Print(" <command>")
	}
	if len(cur.flags) > 0 {
		fmt.Print(" [flags]")
	}
	for _, f := range cur.args {
		n := strings.ToUpper(f.name)
		if f.tag.Default != "" {
			fmt.Printf(" [%s=%s]", n, f.tag.Default)
		} else if f.tag.Req {
			fmt.Printf(" <%s>", n)
		} else {
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
			if l := len(s.name); l > max {
				max = l
			}
		}
		for _, s := range cur.subs {
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
	if a.pre != "" {
		env := a.pre + "CONFIG_FILE"
		fmt.Printf("      --config-file <PATH>    Path to config file [env: %s]\n", env)
	}
	seen := map[string]bool{}
	for _, f := range a.allFlags(cur) {
		if seen[f.name] {
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


