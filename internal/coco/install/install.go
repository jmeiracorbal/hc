package install

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jmeiracorbal/hybrid-coco/internal/coco/manifest"
	"github.com/jmeiracorbal/hybrid-coco/internal/coco/registry"
	"github.com/jmeiracorbal/hybrid-coco/internal/coco/wasmhost"
	"github.com/jmeiracorbal/hybrid-coco/internal/config"
)

// Ref is github.com/owner/repo@version
type Ref struct {
	Module  string
	Version string
}

// ParseRef parses github.com/owner/repo@version.
func ParseRef(s string) (Ref, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Ref{}, fmt.Errorf("module@version is required")
	}
	at := strings.LastIndex(s, "@")
	if at <= 0 || at == len(s)-1 {
		return Ref{}, fmt.Errorf("ref must be module@version")
	}
	mod, ver := s[:at], s[at+1:]
	if !strings.HasPrefix(mod, "github.com/") {
		return Ref{}, fmt.Errorf("module must start with github.com/")
	}
	if ver == "" || ver == "latest" {
		return Ref{}, fmt.Errorf("explicit version is required (latest is not resolved)")
	}
	return Ref{Module: mod, Version: ver}, nil
}

// Result is a successful install.
type Result struct {
	ID       string
	Language string
	Version  string
	Dir      string
}

// FromLocal installs a coco from a directory (builds wasm if needed).
func FromLocal(ctx context.Context, dir string, replace bool) (Result, error) {
	if ctx == nil {
		return Result{}, fmt.Errorf("context is required")
	}
	if dir == "" {
		return Result{}, fmt.Errorf("local directory is required")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Result{}, err
	}
	manPath := filepath.Join(abs, manifest.FileName)
	man, err := manifest.ParseFile(manPath)
	if err != nil {
		return Result{}, err
	}
	running := config.CoreVersion()
	if running == "" {
		return Result{}, config.ErrCoreVersionUnset
	}
	if err := man.Validate(running, manifest.PlatformWASI); err != nil {
		return Result{}, err
	}
	wasmSrc, err := ensureWasm(abs, man.ID)
	if err != nil {
		return Result{}, err
	}
	return finalize(ctx, man, manPath, wasmSrc, man.HCVersion.Version, replace)
}

// FromGitHub downloads release assets and installs.
func FromGitHub(ctx context.Context, ref string, replace bool) (Result, error) {
	if ctx == nil {
		return Result{}, fmt.Errorf("context is required")
	}
	r, err := ParseRef(ref)
	if err != nil {
		return Result{}, err
	}
	parts := strings.Split(strings.TrimPrefix(r.Module, "github.com/"), "/")
	if len(parts) < 2 {
		return Result{}, fmt.Errorf("invalid github module %q", r.Module)
	}
	owner, repo := parts[0], parts[1]
	tag := r.Version
	if !strings.HasPrefix(tag, "v") {
		tag = "v" + tag
	}
	base := fmt.Sprintf("https://github.com/%s/%s/releases/download/%s", owner, repo, tag)

	tmp, err := os.MkdirTemp("", "hc-coco-*")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(tmp)

	manPath := filepath.Join(tmp, manifest.FileName)
	if err := download(ctx, base+"/"+manifest.FileName, manPath); err != nil {
		return Result{}, fmt.Errorf("download coco.toml: %w", err)
	}
	man, err := manifest.ParseFile(manPath)
	if err != nil {
		return Result{}, err
	}
	running := config.CoreVersion()
	if running == "" {
		return Result{}, config.ErrCoreVersionUnset
	}
	if err := man.Validate(running, manifest.PlatformWASI); err != nil {
		return Result{}, err
	}

	wasmName := wasmAssetName(man.ID)
	wasmPath := filepath.Join(tmp, wasmName)
	if err := download(ctx, base+"/"+wasmName, wasmPath); err != nil {
		return Result{}, fmt.Errorf("download %s: %w", wasmName, err)
	}
	return finalize(ctx, man, manPath, wasmPath, r.Version, replace)
}

func finalize(ctx context.Context, man *manifest.Manifest, manSrc, wasmSrc, version string, replace bool) (Result, error) {
	db, err := registry.Open()
	if err != nil {
		return Result{}, err
	}
	defer db.Close()

	if _, err := registry.Get(db, man.ID); err == nil {
		if !replace {
			return Result{}, fmt.Errorf("coco %s already installed; use --replace", man.ID)
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Result{}, err
	}

	sameLang, err := registry.ByLanguage(db, man.Language)
	if err != nil {
		return Result{}, err
	}
	for _, rec := range sameLang {
		if rec.ID == man.ID {
			continue
		}
		if rec.Priority == man.Priority {
			return Result{}, fmt.Errorf("language %q priority tie with %s; change priority or uninstall the other coco", man.Language, rec.ID)
		}
	}

	fixture := []byte("public class Probe {\n  public void run() {}\n}\n")
	if err := wasmhost.Probe(ctx, wasmSrc, "Probe.java", fixture); err != nil {
		return Result{}, fmt.Errorf("abi probe: %w", err)
	}

	destDir, err := registry.DirForID(man.ID)
	if err != nil {
		return Result{}, err
	}
	if err := os.RemoveAll(destDir); err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return Result{}, err
	}
	wasmDest := filepath.Join(destDir, "coco.wasm")
	manDest := filepath.Join(destDir, manifest.FileName)
	if err := copyFile(wasmSrc, wasmDest); err != nil {
		return Result{}, err
	}
	if err := copyFile(manSrc, manDest); err != nil {
		return Result{}, err
	}

	rec := registry.Record{
		ID:           man.ID,
		Language:     man.Language,
		Version:      version,
		Priority:     man.Priority,
		Contract:     man.Contract,
		WasmPath:     wasmDest,
		ManifestPath: manDest,
		InstalledAt:  time.Now().UTC(),
	}
	if err := registry.Upsert(db, rec); err != nil {
		return Result{}, err
	}
	return Result{ID: man.ID, Language: man.Language, Version: version, Dir: destDir}, nil
}

func ensureWasm(dir, id string) (string, error) {
	name := wasmAssetName(id)
	candidates := []string{
		filepath.Join(dir, name),
		filepath.Join(dir, "coco.wasm"),
	}
	for _, c := range candidates {
		if fileExists(c) {
			return c, nil
		}
	}
	out := filepath.Join(dir, name)
	// c-shared produces a WASI reactor with _initialize (required for //go:wasmexport).
	cmd := exec.Command("go", "build", "-buildmode=c-shared", "-o", out, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	outb, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("build wasm: %w\n%s", err, outb)
	}
	return out, nil
}

func wasmAssetName(id string) string {
	base := id
	if i := strings.LastIndex(id, "/"); i >= 0 {
		base = id[i+1:]
	}
	return base + "-wasm32-wasi.wasm"
}

func download(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: HTTP %d", url, res.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, res.Body)
	return err
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// Uninstall removes registry row and install directory.
func Uninstall(id string) error {
	if id == "" {
		return fmt.Errorf("id is required")
	}
	db, err := registry.Open()
	if err != nil {
		return err
	}
	defer db.Close()
	if err := registry.Delete(db, id); err != nil {
		return err
	}
	dir, err := registry.DirForID(id)
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}
