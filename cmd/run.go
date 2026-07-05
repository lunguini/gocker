package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/lunguini/gocker/engine"
	"github.com/urfave/cli/v3"
)

func newRunCmd(eng engine.Runtime) *cli.Command {
	return &cli.Command{
		Name:      "run",
		Usage:     "Run a container",
		ArgsUsage: "IMAGE [COMMAND] [ARG...]",
		// docker is aliased to gocker on many machines (see CLAUDE.md), so
		// third-party scripts pass mainstream Docker flags gocker doesn't
		// implement yet. SkipFlagParsing plus manual parsing lets us accept
		// (and warn-drop) unknown flags instead of hard-erroring like
		// urfave/cli's built-in parser would.
		SkipFlagParsing: true,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			args, interactive, err := parseRunArgs(cmd.Args().Slice())
			if err != nil {
				return err
			}
			return eng.ContainerRun(ctx, args, interactive)
		},
	}
}

// runFlagKind describes whether a flag is boolean or takes a value.
type runFlagKind int

const (
	runFlagBool runFlagKind = iota
	runFlagValue
)

// runFlagSpec describes a supported --run flag: its arity and the argument(s)
// to forward to the backend CLI (empty passArg means special handling).
type runFlagSpec struct {
	kind    runFlagKind
	passArg string
}

// supportedRunFlags mirrors the flags gocker actually understands and knows
// how to translate to the backend `container`/`nerdctl` CLI.
var supportedRunFlags = map[string]runFlagSpec{
	"interactive": {runFlagBool, "-i"},
	"tty":         {runFlagBool, "-t"},
	"detach":      {runFlagBool, "-d"},
	"name":        {runFlagValue, "--name"},
	"volume":      {runFlagValue, "-v"},
	"publish":     {runFlagValue, "-p"},
	"env":         {runFlagValue, "-e"},
	"env-file":    {runFlagValue, ""}, // special-cased: read file, expand to -e
	"workdir":     {runFlagValue, "-w"},
	"rm":          {runFlagBool, "--rm"},
	"network":     {runFlagValue, "--network"},
	"platform":    {runFlagValue, "--platform"},
	"restart":     {runFlagValue, ""}, // special-cased: warn, not forwarded
	"hostname":    {runFlagValue, "--hostname"},
	"cpus":        {runFlagValue, "--cpus"},
	"memory":      {runFlagValue, "-m"},
	"label":       {runFlagValue, "--label"},
}

// runFlagShorts maps single-character short flags to their long name.
var runFlagShorts = map[byte]string{
	'i': "interactive",
	't': "tty",
	'd': "detach",
	'v': "volume",
	'p': "publish",
	'e': "env",
	'w': "workdir",
	'm': "memory",
	'h': "hostname",
	'l': "label",
}

// knownUnsupportedRunFlags are mainstream Docker `run` flags gocker doesn't
// implement yet (Apple Container CLI limitations, or not wired up). They're
// accepted and dropped with a warning rather than causing a hard error, per
// the docker-alias contract in CLAUDE.md. The int is the flag's arity (0 or
// 1) so we consume the right number of following arguments.
var knownUnsupportedRunFlags = map[string]int{
	"user":                1,
	"entrypoint":          1,
	"add-host":            1,
	"pull":                1,
	"cap-add":             1,
	"cap-drop":            1,
	"init":                0,
	"dns":                 1,
	"dns-option":          1,
	"dns-search":          1,
	"detach-keys":         1,
	"health-cmd":          1,
	"health-interval":     1,
	"health-timeout":      1,
	"health-retries":      1,
	"health-start-period": 1,
	"security-opt":        1,
	"privileged":          0,
	"read-only":           0,
	"ipc":                 1,
	"pid":                 1,
	"shm-size":            1,
	"device":              1,
	"expose":              1,
	"link":                1,
	"ulimit":              1,
	"log-driver":          1,
	"log-opt":             1,
	"memory-swap":         1,
	"cpu-shares":          1,
	"sig-proxy":           0,
	"stop-signal":         1,
	"stop-timeout":        1,
	"tmpfs":               1,
	"cidfile":             1,
	"gpus":                1,
}

var knownUnsupportedRunShorts = map[byte]string{
	'u': "user",
}

// parseRunArgs manually parses `gocker run` arguments (SkipFlagParsing is
// enabled), translating supported flags to backend CLI args, warning and
// dropping unknown/unsupported ones, and stopping at the first positional
// argument (the image). It returns the translated backend args and whether
// the run should be treated as interactive.
func parseRunArgs(args []string) ([]string, bool, error) {
	var out []string
	interactive := false
	i := 0

	next := func() (string, bool) {
		i++
		if i < len(args) {
			return args[i], true
		}
		return "", false
	}

	for i < len(args) {
		arg := args[i]

		if arg == "--" {
			i++
			break
		}

		if strings.HasPrefix(arg, "--") {
			name := arg[2:]
			var inlineVal string
			hasInline := false
			if eq := strings.Index(name, "="); eq != -1 {
				inlineVal = name[eq+1:]
				name = name[:eq]
				hasInline = true
			}

			if spec, ok := supportedRunFlags[name]; ok {
				val := inlineVal
				if spec.kind == runFlagValue && !hasInline {
					v, found := next()
					if !found {
						return nil, false, fmt.Errorf("--%s requires a value", name)
					}
					val = v
				}
				switch name {
				case "interactive", "tty":
					interactive = true
				case "env-file":
					envs, err := readEnvFile(val)
					if err != nil {
						return nil, false, err
					}
					for _, e := range envs {
						out = append(out, "-e", e)
					}
					i++
					continue
				case "restart":
					fmt.Fprintf(os.Stderr, "Warning: --restart=%s is not supported by Apple Container CLI (ignored). Container will not auto-restart.\n", val)
					i++
					continue
				}
				if spec.passArg != "" {
					if spec.kind == runFlagBool {
						out = append(out, spec.passArg)
					} else {
						out = append(out, spec.passArg, val)
					}
				}
				i++
				continue
			}

			if arity, ok := knownUnsupportedRunFlags[name]; ok {
				fmt.Fprintf(os.Stderr, "Warning: --%s is not supported by gocker yet (ignored)\n", name)
				if arity == 1 && !hasInline {
					if _, found := next(); !found {
						return nil, false, fmt.Errorf("--%s requires a value", name)
					}
				}
				i++
				continue
			}

			fmt.Fprintf(os.Stderr, "Warning: unknown flag --%s ignored\n", name)
			i++
			continue
		}

		if strings.HasPrefix(arg, "-") && arg != "-" {
			isInteractive, err := parseShortRunFlag(arg, args, &i, &out)
			if err != nil {
				return nil, false, err
			}
			if isInteractive {
				interactive = true
			}
			continue
		}

		// First non-flag token: image name. Stop flag parsing.
		break
	}

	out = append(out, args[i:]...)
	return out, interactive, nil
}

// parseShortRunFlag parses a single "-xyz" token, handling combined boolean
// shorts (e.g. "-it") and value-taking shorts (e.g. "-v /a:/b", "-eFOO=bar").
// It appends translated backend args to out and advances *i past the token
// (and its value, if any). Only one non-boolean short is meaningful per
// token (docker itself doesn't combine value-taking shorts), so the first
// one found terminates the token.
func parseShortRunFlag(arg string, args []string, i *int, out *[]string) (bool, error) {
	interactive := false
	chars := arg[1:]

	for pos := 0; pos < len(chars); pos++ {
		c := chars[pos]

		if long, ok := runFlagShorts[c]; ok {
			spec := supportedRunFlags[long]
			if spec.kind == runFlagBool {
				if long == "interactive" || long == "tty" {
					interactive = true
				}
				if spec.passArg != "" {
					*out = append(*out, spec.passArg)
				}
				continue
			}
			// Value flag: rest of this token (if any) is the inline value,
			// otherwise consume the next argument.
			var val string
			if pos+1 < len(chars) {
				val = chars[pos+1:]
			} else {
				*i++
				if *i >= len(args) {
					return false, fmt.Errorf("-%c requires a value", c)
				}
				val = args[*i]
			}
			if long == "env-file" {
				envs, err := readEnvFile(val)
				if err != nil {
					return false, err
				}
				for _, e := range envs {
					*out = append(*out, "-e", e)
				}
			} else if spec.passArg != "" {
				*out = append(*out, spec.passArg, val)
			}
			*i++
			return interactive, nil
		}

		if long, ok := knownUnsupportedRunShorts[c]; ok {
			fmt.Fprintf(os.Stderr, "Warning: -%c (--%s) is not supported by gocker yet (ignored)\n", c, long)
			// Assume arity 1 for unsupported value shorts (matches -u <user>).
			if pos+1 >= len(chars) {
				*i++
			}
			*i++
			return interactive, nil
		}

		fmt.Fprintf(os.Stderr, "Warning: unknown flag -%c ignored\n", c)
		*i++
		return interactive, nil
	}

	*i++
	return interactive, nil
}

func readEnvFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading env file: %w", err)
	}
	var envs []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Docker semantics: a bare KEY (no '=') inherits the host value
		// and is dropped when the host doesn't have it set.
		if !strings.Contains(line, "=") {
			if val, ok := os.LookupEnv(line); ok {
				envs = append(envs, line+"="+val)
			}
			continue
		}
		envs = append(envs, line)
	}
	return envs, nil
}
