package mcpserver

import (
	"context"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ps2build/ps2build/internal/utils"
	"github.com/ps2build/ps2build/internal/versions"
)

// version is set at build time via -ldflags, same convention as
// internal/cli.version - left as "dev" for plain "go build"/"go run".
var version = "dev"

func SetVersion(v string) {
	if v != "" {
		version = v
	}
}

func New() (*mcp.Server, error) {
	packageRoot := utils.PackageRoot()

	s := mcp.NewServer(&mcp.Implementation{
		Name:    "ps2build",
		Version: version,
	}, nil)

	registerListPackages(s, packageRoot)
	registerDescribePackage(s, packageRoot)
	registerSearchPackages(s, packageRoot)
	registerResolveLibs(s)
	registerGetSchema(s)
	registerExplain(s)
	registerValidateTarget(s)
	registerBuildProject(s)
	registerInstallProject(s, packageRoot)

	return s, nil
}

const updateCheckInterval = 5 * time.Second

func installedPS2BuildTag(sdkRoot string) string {
	f, err := versions.Load(sdkRoot)
	if err != nil {
		return ""
	}
	return f.Components["ps2build"]
}

// exitOnUpdate cancels the returned context once the installed ps2build release differs from the one recorded at startup, so the client restarts the server on the new binary.
func exitOnUpdate(ctx context.Context, sdkRoot string) context.Context {
	started := installedPS2BuildTag(sdkRoot)
	if started == "" {
		return ctx
	}
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		defer cancel()
		t := time.NewTicker(updateCheckInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if now := installedPS2BuildTag(sdkRoot); now != "" && now != started {
					return
				}
			}
		}
	}()
	return ctx
}

func Serve(ctx context.Context) error {
	s, err := New()
	if err != nil {
		return err
	}
	return s.Run(exitOnUpdate(ctx, utils.DetectSDKRoot()), &mcp.StdioTransport{})
}

func ServeHTTP(ctx context.Context, addr string) error {
	s, err := New()
	if err != nil {
		return err
	}
	ctx = exitOnUpdate(ctx, utils.DetectSDKRoot())
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return s
	}, nil)

	httpServer := &http.Server{Addr: addr, Handler: handler}
	errCh := make(chan error, 1)
	go func() { errCh <- httpServer.ListenAndServe() }()

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	}
}
