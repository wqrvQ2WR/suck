<div align="center">
  <h1>🫁 suck</h1>
  <p><strong>Shadow any website to a single binary for offline viewing</strong></p>
  <p>
    <img src="https://img.shields.io/badge/go-1.23%2B-00ADD8?style=flat&logo=go" alt="Go version">
    <img src="https://img.shields.io/badge/license-MIT-green?style=flat" alt="License">
    <img src="https://img.shields.io/github/v/release/wqrvQ2WR/suck?style=flat" alt="Release">
  </p>
  <p>
    <code>suck https://example.com</code> &nbsp;·&nbsp;
    <code>suck ls</code> &nbsp;·&nbsp;
    <code>suck -serve example.com</code>
  </p>
</div>

---

`suck` is a single-binary CLI tool that downloads entire websites for offline viewing — no config, no dependencies, no bullshit. Just **suck** and go.

Inspired by [Kage](https://github.com/tamnd/kage).

## ✨ Features

- **🧠 Automatic** — downloads HTML, rewrites links, fetches same-domain assets (CSS, JS, images, fonts, video)
- **📁 Organized** — saves each site under `~/.suck/<domain>/`
- **🌐 Offline-ready** — built-in HTTP server to browse saved sites locally
- **🖥️ One binary** — Go makes it a tiny, self-contained executable
- **⚡ Fast** — concurrent downloads with zero config

## 📦 Install

```bash
# Option A: Build from source
git clone https://github.com/wqrvQ2WR/suck
cd suck && go build -o suck
sudo cp suck /usr/local/bin/

# Option B: Download a release
# (coming soon)
```

Or just run from anywhere:

```bash
./suck https://example.com
```

## 🚀 Usage

### Download a website

```bash
suck https://example.com          # Full URL
suck example.com                   # https:// is automatic
suck -v example.com                # Verbose — watch files come in
suck -o ~/my-archive example.com   # Custom output directory
```

### Browse saved sites

```bash
suck ls                            # List all saved websites
suck open example.com              # Open saved site in browser
suck -serve example.com            # Serve locally via HTTP
suck -serve -p 3000 example.com    # Custom port
```

### Manage

```bash
suck rm example.com                # Delete a saved site
suck help                          # Show help
```

## 🗂️ Storage

Saved sites live in `~/.suck/<domain>/`:

```
~/.suck/
├── news.ycombinator.com/
│   ├── index.html
│   ├── news.css
│   └── hn.js
├── text.npr.org/
│   ├── index.html
│   ├── nx-s1-5858590
│   └── ...
└── example.com/
    └── index.html
```

## 🧪 Tested with

| Site | Status |
|------|--------|
| `news.ycombinator.com` (CSS + JS + SVG) | ✅ |
| `text.npr.org` (27 linked articles) | ✅ |
| `example.com` | ✅ |

**Works best with:** static sites, documentation, blogs, forums, news sites  
**Limited:** heavy JS-driven SPAs (React, Next.js), sites that require login

## 🛠️ How it works

1. **Fetches** the HTML of the requested URL
2. **Parses** it for same-domain assets (`<link>`, `<script>`, `<img>`, `<video>`, etc.)
3. **Downloads** all assets concurrently
4. **Rewrites** the HTML to point to local files
5. **Saves** everything under `~/.suck/<domain>/`
6. **Serves** it back via `suck -serve` — a built-in zero-config HTTP server

All in ~280 lines of Go.

## 🏗️ Build from source

```bash
git clone https://github.com/wqrvQ2WR/suck
cd suck
go build -o suck main.go
```

## 📄 License

MIT
