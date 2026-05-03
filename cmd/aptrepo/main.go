package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	in := flag.String("in", "dist", "directory containing .deb files")
	out := flag.String("out", "public/apt", "APT repo output directory")
	codename := flag.String("codename", "stable", "distribution codename")
	component := flag.String("component", "main", "APT component")
	flag.Parse()
	pool := filepath.Join(*out, "pool", *component)
	binary := filepath.Join(*out, "dists", *codename, *component, "binary-amd64")
	must(os.MkdirAll(pool, 0755))
	must(os.MkdirAll(binary, 0755))
	var packages strings.Builder
	files, _ := filepath.Glob(filepath.Join(*in, "*.deb"))
	for _, f := range files {
		name := filepath.Base(f)
		dst := filepath.Join(pool, name)
		must(copyFile(f, dst))
		st, err := os.Stat(dst)
		must(err)
		h := mustHash(dst)
		pkg := strings.TrimSuffix(strings.Split(name, "_")[0], ".deb")
		ver := "0.0.0"
		parts := strings.Split(name, "_")
		if len(parts) > 1 {
			ver = parts[1]
		}
		fmt.Fprintf(&packages, "Package: %s\nVersion: %s\nArchitecture: amd64\nMaintainer: Earl Co <earl@aops.studio>\nFilename: pool/%s/%s\nSize: %d\nSHA256: %s\nDescription: Reliable Dropbox sync daemon for Linux and Umbrel\n\n", pkg, ver, *component, name, st.Size(), h)
	}
	pkgPath := filepath.Join(binary, "Packages")
	must(os.WriteFile(pkgPath, []byte(packages.String()), 0644))
	ph := mustHash(pkgPath)
	pst, err := os.Stat(pkgPath)
	must(err)
	release := fmt.Sprintf("Origin: Umbrel Dropbox Client\nLabel: Umbrel Dropbox Client\nSuite: %s\nCodename: %s\nDate: %s\nArchitectures: amd64\nComponents: %s\nSHA256:\n %s %d %s/binary-amd64/Packages\n", *codename, *codename, time.Now().UTC().Format(time.RFC1123Z), *component, ph, pst.Size(), *component)
	must(os.WriteFile(filepath.Join(*out, "dists", *codename, "Release"), []byte(release), 0644))
	fmt.Println("wrote", *out)
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}
func mustHash(p string) string {
	f, err := os.Open(p)
	must(err)
	defer f.Close()
	h := sha256.New()
	_, err = io.Copy(h, f)
	must(err)
	return hex.EncodeToString(h.Sum(nil))
}
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
