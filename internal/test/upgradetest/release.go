//go:build upgrade

package upgradetest

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// oldestRelease is the first release that attaches linux binaries.
const oldestRelease = "v1.2.0"

const releaseURL = "https://github.com/econumo/econumo/releases/download"

type semver struct{ major, minor, patch int }

var releaseTag = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)$`)

func parseSemver(tag string) (semver, bool) {
	m := releaseTag.FindStringSubmatch(tag)
	if m == nil {
		return semver{}, false
	}
	a, _ := strconv.Atoi(m[1])
	b, _ := strconv.Atoi(m[2])
	c, _ := strconv.Atoi(m[3])
	return semver{a, b, c}, true
}

func (v semver) less(o semver) bool {
	if v.major != o.major {
		return v.major < o.major
	}
	if v.minor != o.minor {
		return v.minor < o.minor
	}
	return v.patch < o.patch
}

// sourceReleases picks the newest patch of every minor line from oldestRelease
// on, read from the repository's tags, unless ECONUMO_UPGRADE_FROM lists them.
func sourceReleases(repoRoot string) ([]string, error) {
	if env := strings.TrimSpace(os.Getenv("ECONUMO_UPGRADE_FROM")); env != "" {
		var out []string
		for _, tag := range strings.Split(env, ",") {
			if tag = strings.TrimSpace(tag); tag != "" {
				out = append(out, tag)
			}
		}
		return out, nil
	}
	cmd := exec.Command("git", "tag", "--list", "v*")
	cmd.Dir = repoRoot
	raw, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git tag: %w", err)
	}
	oldest, _ := parseSemver(oldestRelease)
	newest := map[[2]int]semver{}
	for _, tag := range strings.Fields(string(raw)) {
		v, ok := parseSemver(tag)
		if !ok || v.less(oldest) {
			continue
		}
		line := [2]int{v.major, v.minor}
		if cur, ok := newest[line]; !ok || cur.less(v) {
			newest[line] = v
		}
	}
	if len(newest) == 0 {
		return nil, fmt.Errorf("no release tags from %s on; fetch tags (git fetch --tags) or set ECONUMO_UPGRADE_FROM", oldestRelease)
	}
	versions := make([]semver, 0, len(newest))
	for _, v := range newest {
		versions = append(versions, v)
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i].less(versions[j]) })
	out := make([]string, len(versions))
	for i, v := range versions {
		out[i] = fmt.Sprintf("v%d.%d.%d", v.major, v.minor, v.patch)
	}
	return out, nil
}

// releaseBinary returns the path of the release's linux binary for this
// machine's architecture, downloading it into the cache on first use. The
// binary must match the release's SHA256SUMS, cached or not.
func releaseBinary(tag string) (string, error) {
	asset := "econumo-linux-" + runtime.GOARCH
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, tag)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	sums := filepath.Join(dir, "SHA256SUMS")
	bin := filepath.Join(dir, asset)
	if _, err := os.Stat(sums); err != nil {
		if err := download(releaseURL+"/"+tag+"/SHA256SUMS", sums); err != nil {
			return "", err
		}
	}
	want, err := checksumFor(sums, asset)
	if err != nil {
		return "", fmt.Errorf("%s: %w", tag, err)
	}
	if got, err := fileSHA256(bin); err != nil || got != want {
		if err := download(releaseURL+"/"+tag+"/"+asset, bin); err != nil {
			return "", err
		}
		got, err := fileSHA256(bin)
		if err != nil {
			return "", err
		}
		if got != want {
			_ = os.Remove(bin)
			return "", fmt.Errorf("%s/%s: sha256 %s does not match SHA256SUMS %s", tag, asset, got, want)
		}
	}
	if err := os.Chmod(bin, 0o755); err != nil {
		return "", err
	}
	return bin, nil
}

func cacheDir() (string, error) {
	if dir := os.Getenv("ECONUMO_UPGRADE_CACHE"); dir != "" {
		return dir, nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("no cache dir (set ECONUMO_UPGRADE_CACHE): %w", err)
	}
	return filepath.Join(base, "econumo-upgrade"), nil
}

func download(url, dest string) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	tmp := dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return fmt.Errorf("download %s: %w", url, err)
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}

func checksumFor(sumsPath, asset string) (string, error) {
	raw, err := os.ReadFile(sumsPath)
	if err != nil {
		return "", err
	}
	sc := bufio.NewScanner(bytes.NewReader(raw))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == asset {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("SHA256SUMS lists no %s", asset)
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
