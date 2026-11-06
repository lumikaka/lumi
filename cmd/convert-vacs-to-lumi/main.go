// convert-vacs-to-lumi is the offline local converter for downloaded VACS ZIPs.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"lumi/internal/vacsimport"
)

func run(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("convert-vacs-to-lumi", flag.ContinueOnError)
	flags.SetOutput(out)
	output := flags.String("output", "", "new Lumi project directory (must not exist)")
	version := flags.Bool("version", false, "show supported package version")
	flags.Usage = func() {
		fmt.Fprintln(out, `Usage: convert-vacs-to-lumi "我的绘本.vacs.zip" --output "./我的绘本"`)
		flags.PrintDefaults()
	}
	// Standard flag stops at the first positional argument. Support the documented
	// input-first command as well as --output DIR INPUT.
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		args = append(append([]string{}, args[1:]...), args[0])
	}
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if *version {
		fmt.Fprintf(out, "convert-vacs-to-lumi: %s v%d\n", vacsimport.Format, vacsimport.Version)
		return nil
	}
	if flags.NArg() != 1 || *output == "" {
		flags.Usage()
		return fmt.Errorf("one VACS ZIP and --output are required")
	}
	if err := vacsimport.ConvertPackage(ctx, flags.Arg(0), *output, func(n vacsimport.ConversionNotice) {
		fmt.Fprintf(out, "Adjusted display title at %s to %d Unicode characters; original retained in user-data/vacs/manifest.json and conversion-notices.json.\n", n.Location, n.Limit)
	}); err != nil {
		return err
	}
	fmt.Fprintf(out, "Created Lumi project: %s\nOpen this directory with Lumi's Open existing project.\n", *output)
	return nil
}
func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "Conversion failed:", err)
		os.Exit(1)
	}
}
