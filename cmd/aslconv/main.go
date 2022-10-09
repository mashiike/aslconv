package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/mashiike/aslconv"
)

const usage = `aslconv is Amazon State Language(ASL) Format Converter

  usages:
    aslconv -l
    aslconv [options] asl_file
    cat asl_file | aslconv -f json -t hcl

  options:
    -f, --from-formant  original format
    -t, --to-formant    converted format
	-l, --list          displays a list of formats
	-o, --output        output destination. If unspecified, output to stdout
    -h, --help          prints help information
`

func main() {
	if err := _main(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func _main() error {
	var (
		from     string
		to       string
		showList bool
		output   string
	)
	flag.StringVar(&from, "from-formant", "", "")
	flag.StringVar(&from, "f", "", "")
	flag.StringVar(&to, "to-format", "", "")
	flag.StringVar(&to, "t", "", "")
	flag.BoolVar(&showList, "list", false, "")
	flag.BoolVar(&showList, "l", false, "")
	flag.StringVar(&output, "output", "", "")
	flag.StringVar(&output, "o", "", "")
	flag.Usage = func() { fmt.Print(usage) }
	flag.Parse()

	var out io.Writer = os.Stdout
	if output != "" {
		fp, err := os.Create(output)
		if err != nil {
			return err
		}
		defer fp.Close()
		out = fp
	}
	if showList {
		aslconv.ListFormat(out)
		return nil
	}
	if to == "" {
		to = "json"
	}
	toFormat, ok := aslconv.GetFormat(to)
	if !ok {
		return fmt.Errorf("-to-format option: %s is unknown format", to)
	}
	log.Printf("convert to %s", toFormat)
	var asl *aslconv.AmazonStatesLanguage
	if flag.NArg() == 0 {
		if from == "" {
			return errors.New("--from-format or -f option is required, when load from stdin")
		}
		log.Println("load from stdin")
		var err error
		asl, err = aslconv.LoadASLWithReader(os.Stdin, from)
		if err != nil {
			return err
		}
	} else {
		path := flag.Arg(0)
		log.Printf("load from %s", path)
		var err error
		asl, err = aslconv.LoadASLWithPath(path)
		if err != nil {
			return err
		}
	}
	if err := toFormat.WriteASL(out, asl); err != nil {
		return err
	}
	return nil
}
