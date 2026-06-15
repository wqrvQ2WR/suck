# suck - Shadow websites for offline viewing

A single-binary CLI tool that downloads entire websites for offline viewing.
Inspired by [Kage](https://github.com/tamnd/kage).

## Usage

```bash
# Suck a website
suck https://example.com

# Suck (shorthand - no https:// needed)
suck example.com

# Verbose mode
suck -v example.com

# Serve a saved site
suck -serve example.com
```

## How it works

1. Downloads the HTML page
2. Parses and downloads all same-domain assets (CSS, JS, images, videos)
3. Rewrites HTML to reference local files
4. Saves everything in `~/.suck/<domain>/`

## Build from source

```bash
cd ~/capp
go build -o suck main.go
```
