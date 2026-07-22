package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

type Tag struct {
	Help    string
	Short   string
	Default string
	Arg     bool
	Cmd     bool
	Req     bool
	Place   string
}

func parseTag(ft reflect.StructField) Tag {
	t := Tag{}
	for _, s := range strings.Split(ft.Tag.Get("cli"), ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		k, v, _ := strings.Cut(s, "=")
		switch k {
		case "help":
			t.Help = v
		case "short":
			t.Short = v
		case "default":
			t.Default = v
		case "arg":
			t.Arg = true
		case "cmd":
			t.Cmd = true
		case "required":
			t.Req = true
		case "placeholder":
			t.Place = v
		}
	}
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
}

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
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	t := v.Type()
	c := &cmd{name: t.Name(), val: v, parent: parent}
	for i := 0; i < t.NumField(); i++ {
		ft := t.Field(i)
		fv := v.Field(i)
		if !fv.CanSet() {
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

func set(v reflect.Value, s string) {
	switch v.Kind() {
	case reflect.String:
		v.SetString(s)
	case reflect.Bool:
		v.SetBool(s == "true" || s == "1" || s == "")
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n := int64(0)
		fmt.Sscanf(s, "%d", &n)
		v.SetInt(n)
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
		return
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
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

func (a *App) all(r *cmd) []*cmd {
	var w func(*cmd) []*cmd
	w = func(c *cmd) []*cmd {
		x := []*cmd{c}
		for _, s := range c.subs {
			x = append(x, w(s)...)
		}
		return x
	}
	return w(r)
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

func (a *App) setFlag(f *flag, val string, args []string, i *int) {
	if f.val.Kind() == reflect.Bool {
		set(f.val, "true")
		if val == "false" || val == "0" {
			set(f.val, "false")
		}
	} else {
		if val == "" && *i+1 < len(args) {
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
		return fmt.Errorf("invalid flag %q", arg)
	}

	// Combined short flags: -abc (exactly one dash, multiple chars, not =value)
	if arg[0] == '-' && arg[1] != '-' && !strings.Contains(arg, "=") {
		for ci, ch := range arg[1:] {
			rest := arg[ci+2:]
			f := a.flagByName(string(ch), flags)
			if f == nil {
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
				} else if *i+1 < len(args) {
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
	if name == "version" && a.ver != "" {
		fmt.Println(a.ver)
		return errHelp
	}
	f := a.flagByName(name, flags)
	if f != nil {
		a.setFlag(f, val, args, i)
		return nil
	}
	// Unknown flag with suggestion
	var names []string
	for _, f := range flags {
		names = append(names, "--"+f.name)
		if f.tag.Short != "" {
			names = append(names, "-"+f.tag.Short)
		}
	}
	sort.Strings(names)
	msg := fmt.Sprintf("unknown flag %q", arg)
	for _, n := range names {
		nn := strings.TrimLeft(n, "-")
		if nn != "" && levenshtein(strings.TrimLeft(arg, "-"), nn) <= 2 {
			msg += fmt.Sprintf(", did you mean %s?", n)
			break
		}
	}
	return fmt.Errorf("%s", msg)
}

var errHelp = fmt.Errorf("help")

func (a *App) Parse(args []string) error {
	r := a.build(a.root, nil)

	// Find subcommand boundary: first non-flag arg that matches a subcommand
	cmdIdx := 0
	for i, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		for _, s := range r.subs {
			if s.name == arg {
				cmdIdx = i
				goto foundSub
			}
		}
		break
	}
foundSub:

	// Process pre-subcommand flags on root
	rootFlags := a.allFlags(r)
	for i := 0; i < cmdIdx; i++ {
		if err := a.parseOne(args, &i, rootFlags, r); err != nil {
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
		for i, arg := range remain {
			if strings.HasPrefix(arg, "-") {
				continue
			}
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
					pos++
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
			pos++
			continue
		}
		return fmt.Errorf("unexpected argument %q", arg)
	}

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
		if a.prompt && isInteractive() {
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
			return fmt.Errorf("required: %s", strings.Join(missing, ", "))
		}
	}

	// If we ended up at root with subcommands and no Run method, show help
	v := cur.val
	if v.CanAddr() {
		v = v.Addr()
	}
	run := v.MethodByName("Run")
	if !run.IsValid() {
		if cur == r && len(r.subs) > 0 && len(args) == 0 {
			a.help(cur)
			return fmt.Errorf("no command specified")
		}
		return nil
	}
	rargs := []reflect.Value{}
	for i := 0; i < run.Type().NumIn(); i++ {
		t := run.Type().In(i)
		if bv, ok := a.binds[t]; ok {
			rargs = append(rargs, bv)
		} else if t.Kind() == reflect.Ptr {
			rargs = append(rargs, reflect.New(t.Elem()))
		} else {
			rargs = append(rargs, reflect.Zero(t))
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

	var fls []*flag
	for _, f := range cur.flags {
		if f.name != "help" && f.name != "yes" {
			fls = append(fls, f)
		}
	}
	fmt.Println("\nFlags:")
	fmt.Println("  -h, --help    Print help")
	for _, f := range fls {
		b := "  "
		if f.tag.Short != "" {
			b += fmt.Sprintf("-%s, --%s", f.tag.Short, f.name)
		} else {
			b += fmt.Sprintf("    --%s", f.name)
		}
		if f.val.Kind() != reflect.Bool {
			p := f.tag.Place
			if p == "" {
				p = strings.ToUpper(f.name)
			}
			b += fmt.Sprintf(" <%s>", p)
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

func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	d := make([][]int, la+1)
	for i := range d {
		d[i] = make([]int, lb+1)
		d[i][0] = i
	}
	for j := 0; j <= lb; j++ {
		d[0][j] = j
	}
	for i := 1; i <= la; i++ {
		for j := 1; j <= lb; j++ {
			c := 1
			if a[i-1] == b[j-1] {
				c = 0
			}
			d[i][j] = min(d[i-1][j]+1, min(d[i][j-1]+1, d[i-1][j-1]+c))
		}
	}
	return d[la][lb]
}
