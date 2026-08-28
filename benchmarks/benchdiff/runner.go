package benchdiff

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const defaultTimeout = 10 * time.Minute

// Runner 在基线 git worktree 与当前工作树上运行同一组 Go benchmark。
type Runner struct {
	RepoRoot  string
	BaseRef   string
	Go        string
	Git       string
	Bench     string
	Benchtime string
	Packages  []string
	Count     int
	Timeout   time.Duration
}

// Run 执行上一版本与当前工作树的同机 benchmark 对比。
func (r Runner) Run(ctx context.Context) (Report, error) {
	config := r.withDefaults()
	ctx, cancel := context.WithTimeout(ctxOrBackground(ctx), config.Timeout)
	defer cancel()

	repoRoot, err := config.repoRoot(ctx)
	if err != nil {
		return Report{}, err
	}
	baseDir, cleanup, err := config.addBaseWorktree(ctx, repoRoot)
	if err != nil {
		return Report{}, err
	}
	defer cleanup()

	args := config.goTestArgs()
	baseOutput, err := runCommand(ctx, baseDir, config.Go, args, benchEnv(os.Environ()))
	if err != nil {
		return Report{}, fmt.Errorf("base benchmark failed: %w", err)
	}
	candidateOutput, err := runCommand(ctx, repoRoot, config.Go, args, benchEnv(os.Environ()))
	if err != nil {
		return Report{}, fmt.Errorf("candidate benchmark failed: %w", err)
	}
	report := NewReport(config.BaseRef, "working tree", append([]string{config.Go}, args...), baseOutput, candidateOutput)
	if len(report.Comparisons) == 0 {
		return Report{}, ErrNoComparableBenchmarks
	}
	return report, nil
}

func (r Runner) withDefaults() Runner {
	if strings.TrimSpace(r.BaseRef) == "" {
		r.BaseRef = "HEAD"
	}
	if strings.TrimSpace(r.Go) == "" {
		r.Go = "go"
	}
	if strings.TrimSpace(r.Git) == "" {
		r.Git = "git"
	}
	if strings.TrimSpace(r.Bench) == "" {
		r.Bench = "."
	}
	if r.Count <= 0 {
		r.Count = 5
	}
	if r.Timeout <= 0 {
		r.Timeout = defaultTimeout
	}
	return r
}

func (r Runner) repoRoot(ctx context.Context) (string, error) {
	if strings.TrimSpace(r.RepoRoot) != "" {
		return filepath.Abs(r.RepoRoot)
	}
	output, err := runCommand(ctxOrBackground(ctx), "", r.Git, []string{"rev-parse", "--show-toplevel"}, nil)
	if err != nil {
		return "", err
	}
	root := strings.TrimSpace(output)
	if root == "" {
		return "", fmt.Errorf("%w: empty git root", ErrInvalidRunner)
	}
	return filepath.Abs(root)
}

func (r Runner) addBaseWorktree(ctx context.Context, repoRoot string) (string, func(), error) {
	parent, err := os.MkdirTemp("", "gnalloy-benchdiff-*")
	if err != nil {
		return "", nil, err
	}
	baseDir := filepath.Join(parent, "base")
	cleanup := func() {
		_ = exec.Command(r.Git, "-C", repoRoot, "worktree", "remove", "--force", baseDir).Run()
		_ = os.RemoveAll(parent)
	}
	args := []string{"-C", repoRoot, "worktree", "add", "--detach", baseDir, r.BaseRef}
	if _, err := runCommand(ctxOrBackground(ctx), "", r.Git, args, nil); err != nil {
		cleanup()
		return "", nil, err
	}
	return baseDir, cleanup, nil
}

func (r Runner) goTestArgs() []string {
	args := []string{"test", "-run", "^$", "-bench", r.Bench, "-benchmem", "-count", strconvInt(r.Count)}
	if strings.TrimSpace(r.Benchtime) != "" {
		args = append(args, "-benchtime", r.Benchtime)
	}
	return append(args, cleanPackages(r.Packages)...)
}

func cleanPackages(packages []string) []string {
	out := make([]string, 0, len(packages))
	for _, pkg := range packages {
		pkg = strings.TrimSpace(pkg)
		if pkg != "" {
			out = append(out, pkg)
		}
	}
	if len(out) == 0 {
		return []string{"./buffer"}
	}
	return out
}

func runCommand(ctx context.Context, dir string, name string, args []string, env []string) (string, error) {
	effectiveCtx := ctxOrBackground(ctx)
	cmd := exec.CommandContext(effectiveCtx, name, args...)
	if strings.TrimSpace(dir) != "" {
		cmd.Dir = dir
	}
	if env != nil {
		cmd.Env = env
	}
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	if err == nil {
		return output.String(), nil
	}
	if errors.Is(effectiveCtx.Err(), context.DeadlineExceeded) {
		return output.String(), effectiveCtx.Err()
	}
	return output.String(), fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, output.String())
}

func benchEnv(base []string) []string {
	out := setEnv(base, "GOWORK", "off")
	out = setEnv(out, "GOTOOLCHAIN", "local")
	return out
}

func setEnv(env []string, key string, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			continue
		}
		out = append(out, item)
	}
	return append(out, prefix+value)
}

func ctxOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func strconvInt(value int) string {
	return fmt.Sprintf("%d", value)
}
