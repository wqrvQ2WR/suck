package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

var (
	outputDir   string
	port        string
	serveMode   bool
	verbose     bool
	mhtmlMode   bool
	saveName    string
)

func init() {
	home, _ := os.UserHomeDir()
	flag.StringVar(&outputDir, "o", filepath.Join(home, ".suck"), "Output directory")
	flag.StringVar(&port, "p", "8080", "Port for serve mode")
	flag.BoolVar(&serveMode, "serve", false, "Serve saved site")
	flag.BoolVar(&verbose, "v", false, "Verbose output")
	flag.BoolVar(&mhtmlMode, "mhtml", false, "Save as single .mhtml file")
	flag.StringVar(&saveName, "name", "", "Custom save name (default: domain)")
}

func logf(format string, args ...interface{}) {
	if verbose {
		fmt.Fprintf(os.Stderr, format+"\n", args...)
	}
}

func printHelp() {
	fmt.Println(`  suck - Shadow a website for offline viewing

  USAGE:
    suck [options] <url>              Download a website
    suck -serve [options] <domain>     Serve a saved site locally
    suck ls                            List saved websites (folder mode)
    suck rm <domain>                   Delete a saved website
    suck open <domain>                 Open saved site in browser
    suck help                          Show this help

  DOWNLOAD:
    suck https://example.com
    suck example.com                       (https:// is automatic)
    suck -mhtml https://example.com        Save as single .mhtml file
    suck -name mypage https://example.com  Save with custom name
    suck -v example.com                    (verbose mode)
    suck -o /path/to/dir example.com       (custom output dir)

  SERVE:
    suck -serve example.com
    suck -serve -p 3000 example.com        (custom port)

  MANAGE:
    suck ls                          List all saved sites
    suck rm example.com              Delete saved site
    suck open example.com            Open in browser

  OPTIONS:
    -h, --help      Show this help
    -v              Verbose output (show downloaded files)
    -o <dir>        Output directory (default: ~/.suck/)
    -serve          Serve mode (serve a saved site locally)
    -p <port>       Port for serve mode (default: 8080)
    -mhtml          Save as single .mhtml file instead of folder
    -name <name>    Custom save name (default: domain)

  EXAMPLES:
    suck news.ycombinator.com                 # Save HN for offline reading
    suck -mhtml https://namu.wiki/wiki/Go     # Save as .mhtml
    suck -mhtml -name golang wiki.example.com # .mhtml with custom name
    suck ls                                   # See all saved sites
    suck open wiki.example.com                # Open saved site in browser
    suck rm example.com                       # Delete a saved site

  FILES:
    Folder mode: ~/.suck/<domain>/
    MHTML mode:  ~/.suck/<name>.mhtml`)
}

func listSaved() {
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Cannot read %s: %v\n", outputDir, err)
		return
	}

	if len(entries) == 0 {
		fmt.Println("📭 No saved websites yet. Try: suck https://example.com")
		return
	}

	type site struct {
		name string
		time time.Time
		size int64
	}
	var sites []site
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		if e.IsDir() {
			var totalSize int64
			filepath.Walk(filepath.Join(outputDir, e.Name()), func(p string, fi os.FileInfo, err error) error {
				if err == nil && !fi.IsDir() {
					totalSize += fi.Size()
				}
				return nil
			})
			sites = append(sites, site{name: e.Name(), time: info.ModTime(), size: totalSize})
		} else if strings.HasSuffix(e.Name(), ".mhtml") {
			sites = append(sites, site{name: e.Name(), time: info.ModTime(), size: info.Size()})
		}
	}
	sort.Slice(sites, func(i, j int) bool { return sites[i].time.After(sites[j].time) })

	fmt.Println("📂 Saved websites:")
	for _, s := range sites {
		suffix := ""
		if strings.HasSuffix(s.name, ".mhtml") {
			suffix = " (mhtml)"
		}
		fmt.Printf("  %-30s %s  (%s)%s\n", s.name, s.time.Format("Jan _2 15:04"), fmtSize(s.size), suffix)
	}
	fmt.Printf("\n  📁 %s\n", outputDir)
}

func removeSaved(name string) {
	target := filepath.Join(outputDir, name)
	if _, err := os.Stat(target); os.IsNotExist(err) {
		// Try with .mhtml extension
		target = filepath.Join(outputDir, name+".mhtml")
		if _, err := os.Stat(target); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "❌ '%s' not found in %s\n", name, outputDir)
			return
		}
	}
	fmt.Printf("🗑️  Removing %s... ", filepath.Base(target))
	if err := os.RemoveAll(target); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed: %v\n", err)
		return
	}
	fmt.Println("Done!")
}

func openSaved(name string) {
	target := filepath.Join(outputDir, name, "index.html")
	if _, err := os.Stat(target); os.IsNotExist(err) {
		target = filepath.Join(outputDir, name+".mhtml")
		if _, err := os.Stat(target); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "❌ '%s' not found. Suck it first!\n", name)
			return
		}
	}
	absPath, _ := filepath.Abs(target)
	fmt.Printf("🌐 Opening %s...\n", absPath)

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", absPath)
	case "linux":
		cmd = exec.Command("xdg-open", absPath)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", absPath)
	default:
		fmt.Fprintf(os.Stderr, "❌ Unsupported OS: %s\n", runtime.GOOS)
		return
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to open: %v\n", err)
	}
}

func fmtSize(bytes int64) string {
	switch {
	case bytes >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
	case bytes >= 1024:
		return fmt.Sprintf("%.0f KB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func main() {
	flag.Usage = printHelp
	flag.Parse()

	if flag.NArg() == 0 || flag.Arg(0) == "help" || flag.Arg(0) == "--help" || flag.Arg(0) == "-h" {
		printHelp()
		return
	}

	if serveMode {
		serveSaved()
		return
	}

	cmd := flag.Arg(0)
	switch cmd {
	case "ls", "list":
		listSaved()
		return
	case "rm", "remove", "delete":
		if flag.NArg() < 2 {
			fmt.Println("Usage: suck rm <name>")
			os.Exit(1)
		}
		removeSaved(flag.Arg(1))
		return
	case "open":
		if flag.NArg() < 2 {
			fmt.Println("Usage: suck open <name>")
			os.Exit(1)
		}
		openSaved(flag.Arg(1))
		return
	}

	if flag.NArg() < 1 {
		printHelp()
		os.Exit(1)
	}

	rawURL := flag.Arg(0)
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "https://" + rawURL
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid URL: %v\n", err)
		os.Exit(1)
	}

	domain := u.Hostname()
	name := domain
	if saveName != "" {
		name = saveName
	}

	if mhtmlMode {
		savePath := filepath.Join(outputDir, name+".mhtml")
		fmt.Printf("🌐 Sucking %s → %s\n", rawURL, savePath)
		if err := suckMHTML(rawURL, savePath, u); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("\n✅ Saved! Open with:\n   suck open %s\n   Or: open %s\n", name, savePath)
	} else {
		siteDir := filepath.Join(outputDir, name)
		if err := os.MkdirAll(siteDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Cannot create dir: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("🌐 Sucking %s into %s/\n", rawURL, siteDir)
		if err := suckPage(rawURL, siteDir, u); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("\n✅ Saved! Open with:\n   suck -serve %s\n   Or: suck open %s\n", name, name)
	}
}

// ── Folder mode ──────────────────────────────────────────────────────────

func suckPage(pageURL string, siteDir string, baseURL *url.URL) error {
	logf("Fetching %s", pageURL)
	resp, err := httpGet(pageURL)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", pageURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Save original HTML
	savePath := filepath.Join(siteDir, "index.html")
	if err := os.WriteFile(savePath, body, 0644); err != nil {
		return err
	}
	logf("  → saved index.html")

	// Parse and rewrite
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("parse HTML: %w", err)
	}

	downloaded := make(map[string]bool)
	type asset struct {
		url       string
		localPath string
		relPath   string
		node      *html.Node
		attrIdx   int
	}
	var assets []asset

	var walkCollect func(*html.Node)
	walkCollect = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for i, attr := range n.Attr {
				var attrURL string
				switch n.Data {
				case "link":
					if attr.Key == "href" {
						attrURL = attr.Val
					}
				case "img", "script", "source", "iframe", "video", "audio":
					if attr.Key == "src" {
						attrURL = attr.Val
					}
				}

				if attrURL == "" {
					continue
				}
				if strings.HasPrefix(attrURL, "data:") || strings.HasPrefix(attrURL, "javascript:") ||
					strings.HasPrefix(attrURL, "mailto:") || strings.HasPrefix(attrURL, "tel:") ||
					strings.HasPrefix(attrURL, "#") {
					continue
				}

				resolved := resolveURL(attrURL, pageURL)
				if resolved == "" || !isSameDomain(resolved, baseURL.Hostname()) {
					continue
				}
				if downloaded[resolved] {
					continue
				}
				downloaded[resolved] = true

				localPath := urlToPath(resolved, siteDir)
				relPath := localToRel(localPath, siteDir)
				assets = append(assets, asset{url: resolved, localPath: localPath, relPath: relPath, node: n, attrIdx: i})
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walkCollect(c)
		}
	}
	walkCollect(doc)

	// Phase 2: download with progress
	progTotal = len(assets)
	if progTotal > 0 && !verbose {
		renderProgress()
	}
	for _, a := range assets {
		a.node.Attr[a.attrIdx].Val = a.relPath
		downloadAsset(a.url, a.localPath, pageURL, siteDir, downloaded)
	}
	if !verbose && progTotal > 0 {
		fmt.Fprintln(os.Stderr)
	}

	// Write rewritten HTML
	var buf strings.Builder
	if err := html.Render(&buf, doc); err != nil {
		return err
	}
	if err := os.WriteFile(savePath, []byte(buf.String()), 0644); err != nil {
		return err
	}

	return nil
}

// ── MHTML mode ───────────────────────────────────────────────────────────

type mhtmlPart struct {
	url         string
	contentType string
	data        []byte
}

func suckMHTML(pageURL string, savePath string, baseURL *url.URL) error {
	logf("Fetching %s", pageURL)
	resp, err := httpGet(pageURL)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", pageURL, err)
	}
	defer resp.Body.Close()

	htmlData, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Parse HTML
	doc, err := html.Parse(strings.NewReader(string(htmlData)))
	if err != nil {
		return fmt.Errorf("parse HTML: %w", err)
	}

	downloaded := make(map[string]bool)
	var parts []mhtmlPart

	// Collect assets
	var walkCollect func(*html.Node)
	walkCollect = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, attr := range n.Attr {
				var attrURL string
				switch n.Data {
				case "link":
					if attr.Key == "href" {
						attrURL = attr.Val
					}
				case "img", "script", "source", "iframe", "video", "audio":
					if attr.Key == "src" {
						attrURL = attr.Val
					}
				}

				if attrURL == "" {
					continue
				}
				if strings.HasPrefix(attrURL, "data:") || strings.HasPrefix(attrURL, "javascript:") ||
					strings.HasPrefix(attrURL, "mailto:") || strings.HasPrefix(attrURL, "tel:") ||
					strings.HasPrefix(attrURL, "#") {
					continue
				}

				resolved := resolveURL(attrURL, pageURL)
				if resolved == "" || !isSameDomain(resolved, baseURL.Hostname()) {
					continue
				}
				if downloaded[resolved] {
					continue
				}
				downloaded[resolved] = true

				if n.Data == "link" || n.Data == "script" || n.Data == "img" || n.Data == "source" ||
					n.Data == "iframe" || n.Data == "video" || n.Data == "audio" {
					// We'll download after collecting
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walkCollect(c)
		}
	}
	walkCollect(doc)

	// Download all assets
	assetURLs := make([]string, 0, len(downloaded))
	for u := range downloaded {
		// Skip if not a real asset (page links aren't downloaded)
		if strings.Contains(u, "/w/") || strings.Count(u, "/") <= 2 {
			continue // likely a wiki page, not an asset
		}
		assetURLs = append(assetURLs, u)
	}

	progTotal = len(assetURLs) + 1 // +1 for HTML itself
	if !verbose {
		renderProgress()
	}

	// Download HTML as first part
	ct := detectContentType(pageURL, htmlData)
	parts = append(parts, mhtmlPart{url: pageURL, contentType: ct, data: htmlData})
	if !verbose {
		progressInc()
	}

	// Download each asset
	for _, assetURL := range assetURLs {
		data, ct, err := downloadData(assetURL)
		if err != nil {
			logf("  ⚠ %s: %v", assetURL, err)
			if !verbose {
				progressInc()
			}
			continue
		}
		parts = append(parts, mhtmlPart{url: assetURL, contentType: ct, data: data})
		if !verbose {
			progName = extractShortPath(assetURL)
			progressInc()
		}
	}
	if !verbose {
		fmt.Fprintln(os.Stderr)
	}

	// Generate MHTML
	boundary := fmt.Sprintf("suck-boundary-%d", time.Now().UnixNano())
	var mhtmlBuf strings.Builder

	mhtmlBuf.WriteString("From: <saved by suck>\n")
	mhtmlBuf.WriteString("Subject: \n")
	mhtmlBuf.WriteString("Date: " + time.Now().Format(time.RFC1123) + "\n")
	mhtmlBuf.WriteString("MIME-Version: 1.0\n")
	mhtmlBuf.WriteString("Content-Type: multipart/related; boundary=\"" + boundary + "\"\n\n")

	for _, part := range parts {
		mhtmlBuf.WriteString("--" + boundary + "\n")
		mhtmlBuf.WriteString("Content-Type: " + part.contentType + "\n")
		mhtmlBuf.WriteString("Content-Location: " + part.url + "\n")

		if isBinaryContentType(part.contentType) {
			mhtmlBuf.WriteString("Content-Transfer-Encoding: base64\n\n")
			encoded := base64.StdEncoding.EncodeToString(part.data)
			// Split into 76-char lines
			for i := 0; i < len(encoded); i += 76 {
				end := i + 76
				if end > len(encoded) {
					end = len(encoded)
				}
				mhtmlBuf.WriteString(encoded[i:end] + "\n")
			}
		} else {
			mhtmlBuf.WriteString("Content-Transfer-Encoding: quoted-printable\n\n")
			mhtmlBuf.WriteString(string(part.data))
			if !strings.HasSuffix(string(part.data), "\n") {
				mhtmlBuf.WriteString("\n")
			}
		}
		mhtmlBuf.WriteString("\n")
	}
	mhtmlBuf.WriteString("--" + boundary + "--\n")

	if err := os.WriteFile(savePath, []byte(mhtmlBuf.String()), 0644); err != nil {
		return err
	}

	return nil
}

func downloadData(assetURL string) ([]byte, string, error) {
	resp, err := httpGet(assetURL)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	ct := detectContentType(assetURL, data)
	return data, ct, nil
}

func detectContentType(urlStr string, data []byte) string {
	// Try from URL extension first
	ext := strings.ToLower(path.Ext(urlStr))
	switch ext {
	case ".css":
		return "text/css"
	case ".js":
		return "application/javascript"
	case ".json":
		return "application/json"
	case ".svg":
		return "image/svg+xml"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	case ".ttf":
		return "font/ttf"
	case ".otf":
		return "font/otf"
	case ".ico":
		return "image/x-icon"
	case ".html", ".htm":
		return "text/html"
	case ".xml":
		return "text/xml"
	case ".webp":
		return "image/webp"
	case ".avif":
		return "image/avif"
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mp3":
		return "audio/mpeg"
	case ".wasm":
		return "application/wasm"
	}

	// Fall back to MIME sniffing
	ct := http.DetectContentType(data)
	if ct != "" {
		return ct
	}
	return "application/octet-stream"
}

func isBinaryContentType(ct string) bool {
	binaryPrefixes := []string{
		"image/", "video/", "audio/", "font/",
		"application/octet-stream", "application/wasm",
		"application/pdf", "application/zip",
	}
	ctLower := strings.ToLower(ct)
	for _, p := range binaryPrefixes {
		if strings.HasPrefix(ctLower, p) {
			return true
		}
	}
	return false
}

func extractShortPath(assetURL string) string {
	u, _ := url.Parse(assetURL)
	p := u.Path
	if len(p) > 28 {
		p = p[:25] + "..."
	}
	return p
}

// ── Shared helpers ────────────────────────────────────────────────────────

func httpGet(url string) (*http.Response, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "suck/1.0")
	return client.Do(req)
}

func resolveURL(href, base string) string {
	u, err := url.Parse(href)
	if err != nil {
		return ""
	}
	baseU, err := url.Parse(base)
	if err != nil {
		return ""
	}
	return baseU.ResolveReference(u).String()
}

func isSameDomain(rawURL, domain string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "" || host == domain {
		return true
	}
	return strings.HasSuffix(host, "."+domain)
}

func urlToPath(rawURL string, siteDir string) string {
	u, _ := url.Parse(rawURL)
	p := u.Path
	if p == "" || p == "/" {
		p = "/index.html"
	}
	ext := path.Ext(p)
	if ext == "" {
		p = path.Join(p, "index.html")
		ext = ".html"
	}
	dir := filepath.Join(siteDir, filepath.Dir(p))
	filename := filepath.Base(p)
	return filepath.Join(dir, filename)
}

func localToRel(localPath, siteDir string) string {
	rel, err := filepath.Rel(siteDir, localPath)
	if err != nil {
		return filepath.Base(localPath)
	}
	return rel
}

// ── CSS URL parsing ─────────────────────────────────────────────────────

var (
	cssURLRegex  = regexp.MustCompile(`url\((['"]?)([^'")\s]+)(['"]?)\)`)
	cssImportURL = regexp.MustCompile(`@import\s+url\((['"]?)([^'")\s]+)(['"]?)\)`)
	cssImportStr = regexp.MustCompile(`@import\s+['"]([^'"]+)['"]`)
	cssFontFace  = regexp.MustCompile(`src:\s*url\((['"]?)([^'")\s]+)(['"]?)\)`)

	progressMu sync.Mutex
	progDone   int
	progTotal  int
	progName   string
)

func progressAdd(n int) {
	progressMu.Lock()
	defer progressMu.Unlock()
	progTotal += n
	renderProgress()
}

func progressInc() {
	progressMu.Lock()
	defer progressMu.Unlock()
	progDone++
	renderProgress()
}

func renderProgress() {
	if verbose {
		return
	}
	w := 20
	ratio := 0.0
	if progTotal > 0 {
		ratio = float64(progDone) / float64(progTotal)
		if ratio > 1.0 {
			ratio = 1.0
		}
	}
	filled := int(ratio * float64(w))
	bar := strings.Repeat("█", filled) + strings.Repeat("░", w-filled)
	name := progName
	if len(name) > 28 {
		name = name[:25] + "..."
	}
	fmt.Fprintf(os.Stderr, "\r  📦 [%s] %3d/%d  %-28s", bar, progDone, progTotal, name)
	if progDone >= progTotal && progTotal > 0 {
		fmt.Fprintf(os.Stderr, "\r  📦 [%s] %3d/%d  %-28s\n", bar, progDone, progTotal, name)
	}
}

func downloadAsset(assetURL, localPath, pageURL, siteDir string, downloaded map[string]bool) {
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		if !verbose {
			progressInc()
		}
		logf("  ⚠ mkdir: %v", err)
		return
	}

	resp, err := httpGet(assetURL)
	if err != nil {
		if !verbose {
			progressInc()
		}
		logf("  ⚠ fetch %s: %v", assetURL, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if !verbose {
			progressInc()
		}
		logf("  ⚠ %s → HTTP %d", assetURL, resp.StatusCode)
		return
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		if !verbose {
			progressInc()
		}
		logf("  ⚠ read %s: %v", assetURL, err)
		return
	}

	if err := os.WriteFile(localPath, data, 0644); err != nil {
		if !verbose {
			progressInc()
		}
		logf("  ⚠ write %s: %v", localPath, err)
		return
	}

	if verbose {
		logf("  ↓ %s", assetURL)
	} else {
		u, _ := url.Parse(assetURL)
		short := u.Path
		if len(short) > 28 {
			short = short[:25] + "..."
		}
		progressMu.Lock()
		progName = short
		progressMu.Unlock()
		progressInc()
	}

	// Parse CSS for url() references
	if strings.HasSuffix(assetURL, ".css") {
		processCSSRefs(string(data), assetURL, pageURL, siteDir, downloaded)
	}
}

func processCSSRefs(cssContent, cssURL, pageURL, siteDir string, downloaded map[string]bool) {
	seen := make(map[string]bool)
	patterns := []*regexp.Regexp{cssURLRegex, cssImportURL, cssFontFace}

	for _, re := range patterns {
		matches := re.FindAllStringSubmatch(cssContent, -1)
		for _, m := range matches {
			ref := m[2]
			if ref == "" || seen[ref] {
				continue
			}
			seen[ref] = true

			resolved := resolveURL(ref, cssURL)
			if resolved == "" || !isSameDomain(resolved, extractDomain(cssURL)) {
				continue
			}
			if downloaded[resolved] {
				continue
			}
			downloaded[resolved] = true

			localPath := urlToPath(resolved, siteDir)
			downloadAsset(resolved, localPath, pageURL, siteDir, downloaded)
		}
	}

	impMatches := cssImportStr.FindAllStringSubmatch(cssContent, -1)
	for _, m := range impMatches {
		ref := m[1]
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true

		resolved := resolveURL(ref, cssURL)
		if resolved == "" || !isSameDomain(resolved, extractDomain(cssURL)) {
			continue
		}
		if downloaded[resolved] {
			continue
		}
		downloaded[resolved] = true

		localPath := urlToPath(resolved, siteDir)
		downloadAsset(resolved, localPath, pageURL, siteDir, downloaded)
	}
}

func extractDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// ── Serve mode ────────────────────────────────────────────────────────────

func serveSaved() {
	if flag.NArg() < 1 {
		fmt.Println("Usage: suck -serve <domain>")
		os.Exit(1)
	}
	domain := flag.Arg(0)
	siteDir := filepath.Join(outputDir, domain)

	if _, err := os.Stat(siteDir); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "❌ '%s' not found in %s. Suck it first!\n", domain, outputDir)
		os.Exit(1)
	}

	addr := ":" + port
	fmt.Printf("🌐 Serving %s at http://localhost%s\n", domain, addr)
	fmt.Println("   Press Ctrl+C to stop")

	fs := http.FileServer(http.Dir(siteDir))
	http.Handle("/", fs)
	if err := http.ListenAndServe(addr, nil); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Server error: %v\n", err)
		os.Exit(1)
	}
}
