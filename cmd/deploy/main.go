// Command deploy puts the target live on hosts (DESIGN.md, Deploy). Run by
// make deploy [HOST=<host>] in the Mac's clone, it consumes the clone's
// deploy authorization, builds the target from a fresh copy of the public
// repository once for each operating system and architecture among the
// hosts, delivers each host's build to staging/ in its install directory and
// runs the delivered build's install. Host mac is this Mac, delivered to
// locally; every other host is reached with ssh and scp. Without a host it
// deploys every host on the host list, deploy.hosts in the clone.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cehbz/commit-gate/gatestate"
	"github.com/cehbz/commit-gate/release"
)

// hostList is the host list's name at the clone's root, one host per line;
// git ignores it, since the repository is public and names no host.
const hostList = "deploy.hosts"

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	cwd, err := os.Getwd()
	if err != nil {
		return fail("%s", err)
	}
	clone, err := gatestate.Open(cwd)
	if err != nil {
		return fail("%s", err)
	}
	if _, err := os.Stat(clone.DeployTokenPath()); err != nil {
		return fail("no deploy authorization in this clone; the user runs commit-gate approve-deploy here first")
	}
	hosts, err := hostsToDeploy(clone, args)
	if err != nil {
		return fail("%s", err)
	}
	// The run consumes the authorization whether or not every host succeeds.
	if err := os.Remove(clone.DeployTokenPath()); err != nil {
		return fail("%s", err)
	}
	target, err := release.ReadTarget(release.PublicAddress)
	if err != nil {
		return fail("can't read the target: %s", err)
	}
	work, err := os.MkdirTemp("", "commit-gate-deploy-")
	if err != nil {
		return fail("%s", err)
	}
	defer os.RemoveAll(work)
	src := filepath.Join(work, "src")
	if err := freshCopy(target, src); err != nil {
		return fail("%s", err)
	}
	builds := map[platform]string{}
	failed := 0
	for _, h := range hosts {
		if err := deployTo(h, src, work, builds); err != nil {
			fmt.Fprintf(os.Stderr, "deploy: %s: %s\n", h, err)
			failed++
			continue
		}
		fmt.Printf("deploy: %s runs %s (%s)\n", h, target.Tag.Name, target.Commit)
	}
	if failed > 0 {
		return fail("%d of %d hosts failed", failed, len(hosts))
	}
	return 0
}

func fail(format string, a ...any) int {
	fmt.Fprintf(os.Stderr, "deploy: "+format+"\n", a...)
	return 1
}

// hostsToDeploy is the host named on the command line, else every host on
// the clone's host list, one per line, skipping blank lines.
func hostsToDeploy(clone *gatestate.Repo, args []string) ([]string, error) {
	if len(args) > 0 {
		return args, nil
	}
	top, err := clone.Toplevel()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(filepath.Join(top, hostList))
	if err != nil {
		return nil, fmt.Errorf("no HOST and no host list: %w", err)
	}
	defer f.Close()
	var hosts []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if h := strings.TrimSpace(sc.Text()); h != "" {
			hosts = append(hosts, h)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(hosts) == 0 {
		return nil, fmt.Errorf("the host list %s names no host", hostList)
	}
	return hosts, nil
}

// freshCopy checks the target out of a new clone of the public repository
// at dir, so what the Mac's clone holds never reaches a build.
func freshCopy(target release.Target, dir string) error {
	if err := runCmd("git", "clone", "-q", "--no-checkout", release.PublicAddress, dir); err != nil {
		return err
	}
	return runCmd("git", "-C", dir, "checkout", "-q", "--detach", target.Commit)
}

// platform is a host's operating system and architecture, as Go names them.
type platform struct{ goos, goarch string }

// platformOf is this Mac's platform for host mac, and what uname reports
// over ssh for any other host.
func platformOf(host string) (platform, error) {
	if host == release.Mac {
		return platform{runtime.GOOS, runtime.GOARCH}, nil
	}
	probe := exec.Command("ssh", host, "uname -sm")
	probe.Stderr = os.Stderr
	out, err := probe.Output()
	if err != nil {
		return platform{}, fmt.Errorf("ssh %s uname -sm: %w", host, err)
	}
	f := strings.Fields(string(out))
	if len(f) != 2 {
		return platform{}, fmt.Errorf("ssh %s uname -sm: unexpected output %q", host, out)
	}
	goos := map[string]string{"Darwin": "darwin", "Linux": "linux"}[f[0]]
	goarch := map[string]string{"x86_64": "amd64", "amd64": "amd64", "aarch64": "arm64", "arm64": "arm64"}[f[1]]
	if goos == "" || goarch == "" {
		return platform{}, fmt.Errorf("%s runs %s, which the gate is not built for", host, strings.TrimSpace(string(out)))
	}
	return platform{goos, goarch}, nil
}

// deployTo builds the target for host's platform, once per platform,
// delivers it to the host's staging/ and runs its install there.
func deployTo(host, src, work string, builds map[platform]string) error {
	p, err := platformOf(host)
	if err != nil {
		return err
	}
	b, ok := builds[p]
	if !ok {
		b = filepath.Join(work, p.goos+"-"+p.goarch, "commit-gate")
		if err := build(src, b, p); err != nil {
			return err
		}
		builds[p] = b
	}
	if host == release.Mac {
		return deployHere(b)
	}
	return deployOver(host, b)
}

// build builds src's gate for p at out, outside src, so the build records
// the target's commit and an unchanged source.
func build(src, out string, p platform) error {
	cmd := exec.Command("go", "build", "-o", out, "./cmd/cg")
	cmd.Dir = src
	cmd.Env = append(os.Environ(), "GOOS="+p.goos, "GOARCH="+p.goarch, "CGO_ENABLED=0")
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go build for %s/%s: %w", p.goos, p.goarch, err)
	}
	return nil
}

// deployHere delivers build to this Mac's staging/ and runs its install.
func deployHere(build string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	staged := filepath.Join(release.Layout{Home: home}.Staging(), "commit-gate")
	if err := os.MkdirAll(filepath.Dir(staged), 0o755); err != nil {
		return err
	}
	if err := copyFile(build, staged); err != nil {
		return err
	}
	return runCmd(staged, "install")
}

// deployOver delivers build to host's staging/ with scp and runs its
// install over ssh; the paths are relative to the host's home directory.
func deployOver(host, build string) error {
	staging := release.Layout{}.Staging()
	staged := filepath.Join(staging, "commit-gate")
	if err := runCmd("ssh", host, "mkdir -p "+staging); err != nil {
		return err
	}
	if err := runCmd("scp", "-q", build, host+":"+staged); err != nil {
		return err
	}
	return runCmd("ssh", host, staged+" install")
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// runCmd runs a command with its output passed through to stderr.
func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}
