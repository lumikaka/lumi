package mcpbridge

import (
	"context"
	"flag"
	"fmt"
	"io"
	"lumi/internal/platformpath"
	"os"
	"path/filepath"
)

func Command(args []string, in io.Reader, out, errOut io.Writer) error {
	flags := flag.NewFlagSet("lumi --mcp", flag.ContinueOnError)
	flags.SetOutput(errOut)
	environment := flags.String("environment", "production", "production or development")
	dataDir := flags.String("data-dir", "", "Lumi application data directory override")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || (*environment != "production" && *environment != "development") {
		return fmt.Errorf("use --environment production or development")
	}
	if *dataDir == "" {
		var err error
		*dataDir, err = platformpath.DefaultAppDataDir(*environment)
		if err != nil {
			return err
		}
	}
	if !filepath.IsAbs(*dataDir) {
		return fmt.Errorf("data-dir must be absolute")
	}
	token := os.Getenv("LUMI_MCP_TOKEN")
	if token == "" {
		return fmt.Errorf("LUMI_MCP_TOKEN is required; create a project authorization in Lumi")
	}
	return Run(context.Background(), in, out, Path(*dataDir, *environment), *environment, token)
}
