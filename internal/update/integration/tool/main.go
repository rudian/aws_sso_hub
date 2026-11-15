// Command tool is the S0-02 release-side helper and fake GitHub server.
//
//	tool requirement                             prints the designated requirement the app enforces
//	tool keygen   <dir>                          writes <dir>/{current,next}.{priv,pub} (hex)
//	tool manifest -version V -archive F -os darwin -arch arm64 [-notes T] -out F
//	tool sign     -key privfile -in F -out F
//	tool publish  -site D -tag T [-prerelease] file...   adds a release at the top of the list
//	tool serve    -site D -repo o/r -addrfile F          fake GitHub releases API on 127.0.0.1:<random>
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/rudian/codepass/internal/update"
)

func die(err error) {
	fmt.Fprintln(os.Stderr, "tool:", err)
	os.Exit(1)
}

func main() {
	if len(os.Args) < 2 {
		die(fmt.Errorf("usage: tool keygen|manifest|sign|publish|serve"))
	}
	args := os.Args[2:]
	switch os.Args[1] {
	case "requirement":
		fmt.Print(update.DesignatedRequirement(update.BundleID, update.TeamID))
	case "keygen":
		for _, n := range []string{"current", "next"} {
			pub, priv, _ := ed25519.GenerateKey(rand.Reader)
			must(os.WriteFile(filepath.Join(args[0], n+".priv"), []byte(hex.EncodeToString(priv)), 0o600))
			must(os.WriteFile(filepath.Join(args[0], n+".pub"), []byte(hex.EncodeToString(pub)), 0o644))
		}
	case "manifest":
		fs := flag.NewFlagSet("manifest", flag.ExitOnError)
		ver := fs.String("version", "", "")
		arc := fs.String("archive", "", "")
		goos := fs.String("os", "darwin", "")
		arch := fs.String("arch", "arm64", "")
		notes := fs.String("notes", "", "")
		out := fs.String("out", "", "")
		fs.Parse(args)
		b, err := os.ReadFile(*arc)
		must(err)
		sum := sha256.Sum256(b)
		m := update.Manifest{Version: *ver, OS: *goos, Arch: *arch, File: filepath.Base(*arc), Size: int64(len(b)), SHA256: hex.EncodeToString(sum[:]), Notes: *notes}
		j, _ := json.MarshalIndent(m, "", "  ")
		must(os.WriteFile(*out, j, 0o644))
	case "sign":
		fs := flag.NewFlagSet("sign", flag.ExitOnError)
		key := fs.String("key", "", "")
		in := fs.String("in", "", "")
		out := fs.String("out", "", "")
		fs.Parse(args)
		kb, err := os.ReadFile(*key)
		must(err)
		raw, err := hex.DecodeString(strings.TrimSpace(string(kb)))
		must(err)
		data, err := os.ReadFile(*in)
		must(err)
		must(os.WriteFile(*out, []byte(update.SignManifest(ed25519.PrivateKey(raw), data)), 0o644))
	case "publish":
		fs := flag.NewFlagSet("publish", flag.ExitOnError)
		site := fs.String("site", "", "")
		tag := fs.String("tag", "", "")
		pre := fs.Bool("prerelease", true, "")
		fs.Parse(args)
		dir := filepath.Join(*site, *tag)
		must(os.MkdirAll(dir, 0o755))
		for _, f := range fs.Args() {
			b, err := os.ReadFile(f)
			must(err)
			must(os.WriteFile(filepath.Join(dir, filepath.Base(f)), b, 0o644))
		}
		order := readOrder(*site)
		order = append([]string{*tag}, order...)
		must(os.WriteFile(filepath.Join(*site, "order.txt"), []byte(strings.Join(order, "\n")+"\n"), 0o644))
		_ = pre
	case "serve":
		fs := flag.NewFlagSet("serve", flag.ExitOnError)
		site := fs.String("site", "", "")
		repo := fs.String("repo", "acme/codepass-test", "")
		addrfile := fs.String("addrfile", "", "")
		fs.Parse(args)
		serve(*site, *repo, *addrfile)
	default:
		die(fmt.Errorf("unknown command"))
	}
}

func must(err error) {
	if err != nil {
		die(err)
	}
}

func readOrder(site string) []string {
	b, _ := os.ReadFile(filepath.Join(site, "order.txt"))
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

type asset struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Size int64  `json:"size"`
	URL  string `json:"url"`
}
type release struct {
	TagName    string  `json:"tag_name"`
	Name       string  `json:"name"`
	Body       string  `json:"body"`
	Prerelease bool    `json:"prerelease"`
	Draft      bool    `json:"draft"`
	Assets     []asset `json:"assets"`
}

func serve(site, repo, addrfile string) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	must(err)
	base := "http://" + ln.Addr().String()
	build := func() []release {
		var out []release
		for i, tag := range readOrder(site) {
			r := release{TagName: tag, Name: tag, Prerelease: true,
				// A hostile release body: must never reach the UI.
				Body: "UNSIGNED NOTES FROM THE API (must never be shown)"}
			ents, _ := os.ReadDir(filepath.Join(site, tag))
			for j, e := range ents {
				fi, _ := e.Info()
				r.Assets = append(r.Assets, asset{ID: int64(i*100 + j + 1), Name: e.Name(), Size: fi.Size(),
					URL: base + "/repos/" + repo + "/releases/assets/" + tag + "/" + e.Name()})
			}
			out = append(out, r)
		}
		return out
	}
	mux := http.NewServeMux()
	// Repository-access check used by the non-production channel provider.
	mux.HandleFunc("/repos/"+repo, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"full_name": repo})
	})
	mux.HandleFunc("/repos/"+repo+"/releases", func(w http.ResponseWriter, r *http.Request) {
		if _, err := os.Stat(filepath.Join(site, "DOWN")); err == nil {
			http.Error(w, "boom", 500)
			return
		}
		json.NewEncoder(w).Encode(build())
	})
	mux.HandleFunc("/repos/"+repo+"/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		rs := build()
		if len(rs) == 0 {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(rs[0])
	})
	prefix := "/repos/" + repo + "/releases/assets/"
	mux.HandleFunc(prefix, func(w http.ResponseWriter, r *http.Request) {
		// Like GitHub: the API asset URL redirects to a CDN host/path.
		http.Redirect(w, r, "/cdn/"+strings.TrimPrefix(r.URL.Path, prefix), http.StatusFound)
	})
	mux.HandleFunc("/cdn/", func(w http.ResponseWriter, r *http.Request) {
		rel := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/cdn/"))
		if strings.HasPrefix(rel, "..") {
			http.NotFound(w, r)
			return
		}
		f, err := os.Open(filepath.Join(site, rel))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		w.Header().Set("Content-Type", "application/octet-stream")
		io.Copy(w, f)
	})
	must(os.WriteFile(addrfile, []byte(base), 0o644))
	must(http.Serve(ln, mux))
}
