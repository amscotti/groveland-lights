package web

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"path/filepath"
	"sort"

	assets "github.com/amscotti/groveland-lights"
)

// templates holds every parsed page and partial. Pages render through base.html;
// partials render bare (used by HTMX).
type templates struct {
	pages    map[string]*template.Template
	partials map[string]*template.Template
}

// parseTemplates walks web/templates: any file whose first dir is pages/ is a
// full page (rendered with base.html); anything under partials/ renders bare.
func parseTemplates() (*templates, error) {
	t := &templates{pages: map[string]*template.Template{}, partials: map[string]*template.Template{}}
	root := assets.Templates()
	var files []string
	err := fs.WalkDir(root, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(p) == ".html" {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	for _, f := range files {
		switch {
		case f == "base.html":
			continue
		case isPage(f):
			tt, err := template.ParseFS(root, "base.html", f)
			if err != nil {
				return nil, fmt.Errorf("page %s: %w", f, err)
			}
			t.pages[f] = tt
		default:
			tt, err := template.ParseFS(root, f)
			if err != nil {
				return nil, fmt.Errorf("partial %s: %w", f, err)
			}
			t.partials[f] = tt
		}
	}
	if _, ok := t.pages["pages/index.html"]; !ok {
		return nil, fmt.Errorf("missing pages/index.html")
	}
	return t, nil
}

func isPage(p string) bool {
	dir := filepath.ToSlash(filepath.Dir(p))
	return dir == "pages" || dir == "." || dir == "/"
}

// renderPage executes a page through base.html.
func (t *templates) renderPage(w io.Writer, page string, data any) error {
	tpl, ok := t.pages[filepath.ToSlash("pages/"+filepath.Base(page))]
	if !ok {
		return fmt.Errorf("unknown page %s", page)
	}
	var buf bytes.Buffer
	if err := tpl.ExecuteTemplate(&buf, "base.html", data); err != nil {
		return err
	}
	_, err := w.Write(buf.Bytes())
	return err
}
