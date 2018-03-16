package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/gobs/args"
	"github.com/gobs/sortedmap"
)

/*
var = value

target ... : prerequisite ...
    recipe
    ...
*/

type State int

const (
	SStart State = iota
	SSetting
	STarget
	SRecipe
)

type Target struct {
	name    string
	prereq  []string
	recipes []string
	tstamp  time.Time
}

func (t *Target) String() string {
	return fmt.Sprintf("{target: %q, prereq: %q, recipes: %q}", t.name, t.prereq, t.recipes)

}

func (t *Target) expandRecipe(r int) (recipe string, silent bool) {
	recipe = t.recipes[r]
	if strings.HasPrefix(recipe, "@") {
		recipe = strings.TrimSpace(recipe[1:])
		silent = true
	}

	prereq := ""
	if len(t.prereq) > 0 {
		prereq = t.prereq[0]
	}

	recipe = strings.Replace(recipe, "$@", t.name, -1)
	recipe = strings.Replace(recipe, "$<", prereq, -1)
	return
}

type Maker struct {
	targets map[string]*Target // targets
	start   string             // main target
	debug   bool
	dryrun  bool
	ignore  bool
	keep    bool
	silent  bool
}

func (m *Maker) AddTargets(targets, prereq, recipes []string) {
	if m.start == "" {
		m.start = targets[0]
	}

	for _, t := range targets {
		if _, ok := m.targets[t]; ok {
			log.Fatalf("target %q already exists\n", t)
		}

		m.targets[t] = &Target{name: t, prereq: prereq, recipes: recipes, tstamp: modTime(t)}
	}
}

func (m *Maker) Process(target string, now time.Time) {
	if target == "" {
		target = m.start
	}

	t := m.targets[target]
	if t == nil {
		mtime := modTime(target)
		if mtime.IsZero() {
			log.Fatalf("unknown target %q\n", target)
			return
		}

		t = &Target{name: target, tstamp: mtime}
		m.targets[target] = t
	}

	if t.tstamp.After(now) {
		log.Println("nothing to do for", target)
		return
	}

	if m.debug {
		log.Printf("target %q\n", target)
		if len(t.prereq) > 0 {
			log.Printf("  dependencies: %q\n", t.prereq)
		}
	}

	for _, p := range t.prereq {
		m.Process(p, now)
	}

	for r := range t.recipes {
		recipe, silent := t.expandRecipe(r)
		silent = silent || m.silent

		if m.debug {
			log.Printf("  run %q\n", recipe)
		} else if !silent {
			fmt.Println(recipe)
		}

		if m.dryrun {
			continue
		}

		ignore := m.ignore
		if strings.HasPrefix(recipe, "-") {
			recipe = strings.TrimSpace(recipe[1:])
			ignore = true
		}

		if err := runCommand(recipe); err != nil && !ignore {
			log.Fatal(err)
		}
	}

	t.tstamp = time.Now()
}

func readMakefile(mfile string) (maker *Maker) {
	f, err := os.Open(mfile)
	if err != nil {
		log.Fatal(err)
	}

	defer f.Close()

	maker = &Maker{targets: map[string]*Target{}}

	scanner := bufio.NewScanner(f)
	state := SStart

	var targets, prereq, recipes []string

	addTargets := func() {
		if state == SRecipe {
			maker.AddTargets(targets, prereq, recipes)
			targets = nil
			prereq = nil
			recipes = nil
		}

		state = SStart
	}

	for scanner.Scan() {
		line := scanner.Text()

		// merge continuation lines
		for strings.HasSuffix(line, `\`) && !strings.HasSuffix(line, `\\`) {
			line = trimContinuation(line)

			if scanner.Scan() {
				next := strings.TrimSpace(scanner.Text())
				line += " " + next
			} else {
				break
			}
		}

		line = expandVariables(line)
		parts := cleanArguments(args.GetArgs(line, args.UserTokens("=:")))

		//
		// empty line
		// finish up and reset the state
		//
		if len(parts) == 0 {
			if state == SRecipe {
				addTargets()
			}

			continue
		}

		//
		// variable assignment
		//
		if state == SStart && len(parts) >= 2 && parts[1] == "=" {
			vars[parts[0]] = strings.Join(parts[2:], " ")
			continue
		}

		indented := line[0] == ' ' || line[0] == '\t'

		switch state {
		case SRecipe:
			if indented {
				// XXX: line may still have comments in it
				recipes = append(recipes, strings.TrimSpace(line))
			} else {
				addTargets()
				goto process_sstart
			}
			break

		process_sstart:
			fallthrough

		case SStart:
			if indented {
				fatalError("unexpected indentation", line)
			}

			targets, prereq = parseTargets(parts)
			if targets == nil {
				fatalError("invalid target", line)
			}

			state = SRecipe
		}
	}

	addTargets()

	if scanner.Err() != nil {
		log.Fatal(scanner.Err())
	}

	return
}

func main() {
	mfile := flag.String("f", "Makefile", "make file")
	targets := flag.Bool("targets", false, "print available targets")
	version := flag.Bool("v", false, "print version and exit")
	debug := flag.Bool("d", false, "debug logging")
	dryrun := flag.Bool("n", false, "dry-run - print steps but don't execute them")
	ignore := flag.Bool("i", false, "ignore errors")
	keep := flag.Bool("k", false, "keep going")
	silent := flag.Bool("s", false, "silent")

	flag.Parse()

	if *version {
		printVersion()
		return
	}

	maker := readMakefile(*mfile)
	maker.debug = *debug
	maker.dryrun = *dryrun
	maker.ignore = *ignore
	maker.keep = *keep
	maker.silent = *silent

	if *targets {
		fmt.Println("\nAvailable targets:")

		for _, t := range sortedmap.AsSortedMap(maker.targets) {
			fmt.Println("   ", t.Key)
		}

		return
	}

	if flag.NArg() == 0 {
		maker.Process("", time.Now())
	} else {
		for _, target := range flag.Args() {
			maker.Process(target, time.Now())
		}
	}
}

var reVar = regexp.MustCompile(`\$(\w+|\(\w+\)|\(ENV.\w+\))`) // $var or $(var)
var vars = map[string]string{}

func expandVariables(line string) string {
	for {
		// fmt.Println("before expand:", line)
		found := false

		line = reVar.ReplaceAllStringFunc(line, func(s string) string {
			found = true

			// ReplaceAll doesn't return submatches so we need to cleanup
			arg := strings.TrimLeft(s, "$(")
			arg = strings.TrimRight(arg, ")")

			if strings.HasPrefix(arg, "ENV.") {
				return os.Getenv(arg[4:])
			}

			return vars[arg]
		})

		// fmt.Println("after expand:", line)
		if !found {
			break
		}
	}

	return line
}

func trimContinuation(line string) string {
	line = strings.TrimRight(line, `\`)
	line = strings.TrimRightFunc(line, unicode.IsSpace)
	return line
}

func cleanArguments(args []string) (ret []string) {
	for _, a := range args {
		if strings.HasPrefix(a, "#") { // stop at comments
			break
		}

		if a != "" { // remove empty parts
			ret = append(ret, a)
		}
	}

	return
}

func fatalError(message, line string) {
	log.Fatalf("%v near %q\n", message, line)
}

func parseTargets(parts []string) (targets []string, prereq []string) {
	for i, p := range parts {
		if p = strings.TrimSpace(p); p == ":" {
			prereq = parts[i+1:]
			return
                }

                targets = append(targets, p)
	}

	return nil, nil
}

func modTime(target string) (tstamp time.Time) {
	fi, err := os.Lstat(target)
	if err != nil {
		// log.Println(err)
		return
	}

	return fi.ModTime()
}

func printVersion() {
	version := `
	        .*,,..,///(
	        .///#(/#(((
	        ./*  **/(/#.
	         *///&&&&%%
	         (*, /%%%*.
	         #*. *(%#*,
	         %*. *(%%//
	        ,#*. ,(#(/(
	        #%/*(/&%%%%*
	       /#&((#(&&/%            ________        __
	      *%%%%(##&&(&(%#        /  _____/  ____ |  | __ ____
	      &%&/####&&/%%#%*      /   \  ___ /  _ \|  |/ // __ \
	     %##%(((%(%%&*#/  %     \    \_\  (  <_> )    <\  ___/
	    /#((%*%&%(&%#%&    &     \______  /\____/|__|_ \\___  > v. 0.1
	   .&&%(#(((&/%&%#(%    :           \/            \/    \/
	   .&&&&&&&&&&&&&&((*,  .
	   (%(#*&,  #&&&/((%((/#.
	  ./#(/%(*(##(/#%#(/(/,
	  /*#(#*(/(%###%#%(%#%#/*
	  ##(%%#/(%@@(#%##&((/(#/
	  #(%&&&&&@@@@@@@@@@&&%//
	  (&%##(#/*@#%%%%##%*   /
	  ##%%&@@@@@&&&&&&%((%* /
	  #&&&@&%##@@%%@@@%@@@%%/
	  .&@&@@&%#@@&&@@@%@@@%#/
	   &@%@@&%%@@&&@@@%@@%%%.
	   (@%@@&%%@@&@@@@&@@%%&
	   ,@&@@&%%@@&@@@@&@&%%&
	    @@@@&%%@@&@@@@@@%%%#
	    &@&@@%%@&&@@@@@@%%%*
	    &@&@@%%@&&@@@@@@%#&.
	    &@&@&%%@&&@@@@@&%%&*
	   /@@&&@%%@&&@@@@&&&@/&
	   &&&@&&.%@&@@@@@&&&@&(,
	  (&&&&&*%&&&&&@@@@&&%%%%
	  (%&&%&&&%%&&%%%%@@##/ &
	  //(%&&&&&%&###%%%%#** )
	   */// ,(######(((%#&%(*
	`

	fmt.Println(strings.Replace(version, "\t", "", -1))
}
