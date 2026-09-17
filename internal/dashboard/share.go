package dashboard

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

// A public view is a separate allowlisted projection, never a localUsage response.
type publicUsage struct {
	Month, Updated string
	Tokens, Calls  int64
	CacheRate      string
	Models         []modelUsage
}

type publicSnapshot struct {
	mu     sync.RWMutex
	assets map[string][]byte // Immutable published variants; HTTP reads never render or fetch.
}

var publicComponents = map[string]string{"overview": "公开用量", "tokens": "总 Token", "calls": "调用次数", "cache": "缓存命中率", "models": "模型用量"}
var hubComponents = []string{"tokens", "calls", "cache", "models"}

func componentMask(text string) (int, error) {
	mask := 0
	for name := range strings.SplitSeq(text, ",") {
		i := slices.Index(hubComponents, name)
		if i < 0 || mask&(1<<i) != 0 {
			return 0, errors.New("components must be unique: tokens,calls,cache,models")
		}
		mask |= 1 << i
	}
	return mask, nil
}

func hubPath(mask int) string {
	var names []string
	for i, name := range hubComponents {
		if mask&(1<<i) != 0 {
			names = append(names, name)
		}
	}
	return "hub/" + strings.Join(names, "-")
}

type publicPart struct {
	Key, Title, Value string
	X, Y              int
}

type publicRow struct {
	modelUsage
	Y int
}

type publicView struct {
	publicUsage
	Component, Theme, Title, Value string
	Width, Height, RowsY           int
	Rows                           []publicRow
	Parts                          []publicPart
	Columns                        int
}

func compactCount(n int64) string {
	for _, unit := range []struct {
		size   int64
		suffix string
	}{{1_000_000_000, "B"}, {1_000_000, "M"}, {1_000, "K"}} {
		if n >= unit.size {
			value := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", float64(n)/float64(unit.size)), "0"), ".")
			return value + unit.suffix
		}
	}
	return fmt.Sprint(n)
}

//go:embed public/resize.js
var resizeScript string
var resizeHash = func() string {
	digest := sha256.Sum256([]byte(resizeScript))
	return "'sha256-" + base64.StdEncoding.EncodeToString(digest[:]) + "'"
}()

var shareTemplate = template.Must(template.New("share.html").Funcs(template.FuncMap{
	"resizeScript": func() template.JS { return template.JS(resizeScript) },
	"compact":      compactCount,
	"digits":       func(n int64) []string { return strings.Split(compactCount(n), "") },
	"short": func(s string) string {
		r := []rune(s)
		if len(r) > 28 {
			return string(r[:27]) + "…"
		}
		return s
	},
}).ParseFS(webFiles, "public/share.html", "public/share.svg"))

func monthlyPublicUsage(data *snapshot, loc *time.Location) publicUsage {
	updated := data.FetchedAt.In(loc)
	start := time.Date(updated.Year(), updated.Month(), 1, 0, 0, 0, 0, updated.Location()).UnixMilli()
	end := updated.UnixMilli()
	view := publicUsage{Month: updated.Format("2006.01"), Updated: updated.Format("2006-01-02 15:04 MST"), CacheRate: "—"}
	models := map[string]modelUsage{}
	var input, cached int64
	for _, session := range data.Files {
		for _, event := range session.Events {
			if event.Timestamp < start || event.Timestamp >= end {
				continue
			}
			view.Tokens += event.Tokens
			view.Calls++
			input += event.Input + event.Cached + event.Write
			cached += event.Cached
			model := models[event.Model]
			model.Model = event.Model
			model.Tokens += event.Tokens
			model.Calls++
			models[event.Model] = model
		}
	}
	if input > 0 {
		view.CacheRate = fmt.Sprintf("%.1f%%", float64(cached)/float64(input)*100)
	}
	for _, model := range models {
		view.Models = append(view.Models, model)
	}
	slices.SortFunc(view.Models, func(a, b modelUsage) int {
		return cmp.Or(cmp.Compare(b.Tokens, a.Tokens), strings.Compare(a.Model, b.Model))
	})
	return view
}

func (p *publicSnapshot) publish(data *snapshot) error {
	assets, err := renderPublicAssets(monthlyPublicUsage(data, time.Local), 15)
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.assets = assets
	p.mu.Unlock()
	return nil
}

func renderPublicAssets(view publicUsage, allowed int) (map[string][]byte, error) {
	views := make(map[string]publicView)
	for component, title := range publicComponents {
		mask := 15
		if component != "overview" {
			mask, _ = componentMask(component) // Keys come from the fixed component catalog.
		}
		if mask&allowed != mask {
			continue
		}
		v := publicView{publicUsage: view, Component: component, Title: title, Width: 480, Height: 180}
		switch component {
		case "tokens":
			v.Value = compactCount(view.Tokens)
		case "calls":
			v.Value = fmt.Sprint(view.Calls)
		case "cache":
			v.Value = view.CacheRate
		case "models", "overview":
			v.Width = 640
			top := 84
			if component == "overview" {
				top = 290
			}
			v.RowsY = top
			for i, model := range view.Models {
				v.Rows = append(v.Rows, publicRow{modelUsage: model, Y: top + i*38})
			}
			v.Height = top + max(1, len(v.Rows))*38 + 50
		}
		views[component] = v
	}
	// Four optional components give 15 bounded combinations. Publish them with
	// the other snapshots so anonymous reads never build views or scan sessions.
	for mask := 1; mask < 1<<len(hubComponents); mask++ {
		if mask&allowed != mask {
			continue
		}
		v := publicView{publicUsage: view, Component: "hub", Title: "用量", Height: 210}
		for i, component := range hubComponents {
			if mask&(1<<i) == 0 {
				continue
			}
			v.Parts = append(v.Parts, publicPart{Key: component, Title: publicComponents[component], Value: views[component].Value})
			if component != "models" {
				v.Columns++
			}
		}
		v.Width = max(480, v.Columns*260)
		if mask&8 != 0 {
			v.Width = max(640, v.Width)
		}
		for i := range v.Parts {
			part := &v.Parts[i]
			part.X, part.Y = 24+i*v.Width/max(1, v.Columns), 70
			if part.Key == "models" {
				part.X, part.Y = 0, 70
				if v.Columns > 0 {
					part.Y = 210
				}
				v.RowsY = 56
				for j, model := range view.Models {
					v.Rows = append(v.Rows, publicRow{modelUsage: model, Y: v.RowsY + j*38})
				}
				v.Height = part.Y + v.RowsY + max(1, len(v.Rows))*38 + 50
			}
		}
		v.Columns = max(1, v.Columns)
		views[hubPath(mask)] = v
	}
	assets := make(map[string][]byte, len(views)*6)
	for key, view := range views {
		for _, theme := range []string{"auto", "light", "dark"} {
			v := view
			v.Theme = theme
			for _, format := range []string{"html", "svg"} {
				var output bytes.Buffer
				if err := shareTemplate.ExecuteTemplate(&output, "share."+format, v); err != nil {
					return nil, err
				}
				assets[key+"/"+theme+"."+format] = output.Bytes()
			}
		}
	}
	return assets, nil
}

func (a *application) serveShare(w http.ResponseWriter, r *http.Request) {
	if !a.config.Share {
		sendError(w, http.StatusNotFound, "公开展示未启用")
		return
	}
	component, format := r.PathValue("component"), "html"
	if component == "embed.js" {
		raw, err := webFiles.ReadFile("public/embed.js")
		if err != nil {
			sendError(w, 500, "嵌入脚本不可用")
			return
		}
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=300")
		if r.Method != http.MethodHead {
			_, _ = w.Write(raw)
		}
		return
	}
	if component == "" {
		component = "overview"
	}
	if name, ok := strings.CutSuffix(component, ".svg"); ok {
		component, format = name, "svg"
	}
	if _, ok := publicComponents[component]; !ok && component != "hub" {
		sendError(w, http.StatusNotFound, "公开组件不存在")
		return
	}
	key := component
	if component == "hub" {
		selected := "tokens,calls,cache,models"
		if r.URL.Query().Has("components") {
			selected = r.URL.Query().Get("components")
		}
		if len(selected) > 64 {
			sendError(w, http.StatusBadRequest, "组件组合无效")
			return
		}
		mask, err := componentMask(selected)
		if err != nil {
			sendError(w, http.StatusBadRequest, "组件组合无效")
			return
		}
		key = hubPath(mask)
	}
	theme := cmp.Or(r.URL.Query().Get("theme"), "auto")
	if theme != "auto" && theme != "light" && theme != "dark" {
		sendError(w, http.StatusBadRequest, "不支持的主题")
		return
	}
	p := a.local.public
	p.mu.RLock()
	page := p.assets[key+"/"+theme+"."+format]
	p.mu.RUnlock()
	if page == nil {
		w.Header().Set("Retry-After", "60")
		sendError(w, http.StatusServiceUnavailable, "公开快照尚未生成")
		return
	}
	// Only these public components permit embedding; private routes still deny framing.
	w.Header().Del("X-Frame-Options")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors *; base-uri 'none'; form-action 'none'; sandbox")
	if format == "html" {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src "+resizeHash+"; style-src 'unsafe-inline'; frame-ancestors *; base-uri 'none'; form-action 'none'; sandbox allow-scripts")
	}
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.Header().Set("Cache-Control", "public, max-age=300")
	contentType := "text/html; charset=utf-8"
	if format == "svg" {
		contentType = "image/svg+xml"
	}
	w.Header().Set("Content-Type", contentType)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(page)
}
