package main

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/nsgo-dev/nsgo/core"
)

func isImagePath(path string) bool {
	name := filepath.Base(path)
	ext := filepath.Ext(name)
	if len(ext) == len(name) {
		return false
	}
	switch strings.ToLower(ext) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".ico", ".svg":
		return true
	default:
		return false
	}
}

func (w *workbench) imageDiff(cx *core.Scope, chrome palette, file fileDiff) core.Node {
	oldLabel, newLabel := w.tr("Old", "旧版本"), w.tr("New", "新版本")
	repo := w.core.repo
	loadBlob := func(oid string) func(context.Context) ([]byte, error) {
		if oid == "" {
			return nil
		}
		return func(ctx context.Context) ([]byte, error) { return repo.Image(ctx, oid) }
	}
	loadOld, loadNew := loadBlob(file.OldSHA), loadBlob(file.NewSHA)
	if file.WorkTreePath != "" {
		loadNew = func(ctx context.Context) ([]byte, error) { return repo.WorkingImage(ctx, file.WorkTreePath) }
	}
	old := w.imageSide(cx, chrome, loadOld, oldLabel, w.tr("Added", "新增"))
	newVersion := w.imageSide(cx, chrome, loadNew, newLabel, w.tr("Deleted", "已删除"))
	style := core.Style{FlexGrow: 1, FlexShrink: 1, Direction: core.Row, Gap: 1, Background: chrome.border}
	if loadOld == nil || loadNew == nil {
		style.Direction = core.Column
	}
	return core.Scroll{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)},
		Children: []core.Node{core.View{Style: style, Children: []core.Node{old, newVersion}}}}
}

func (w *workbench) imageSide(cx *core.Scope, chrome palette, load func(context.Context) ([]byte, error), label, missing string) core.Node {
	var body core.Node
	if load == nil {
		body = notice(chrome, missing, "", nil)
	} else {
		raw := core.NewSignalFunc(cx, []byte(nil), nil)
		message := core.NewSignal(cx, w.tr("Loading…", "加载中…"))
		ctx := cx.Context()
		go func() {
			image, err := load(ctx)
			cx.Post(func() {
				if err != nil || len(image) == 0 {
					message.Set(w.tr("Unable to display this image", "无法显示该图片"))
				} else {
					raw.Set(image)
				}
			})
		}()
		body = core.Dynamic(func(*core.Scope) core.Node {
			bytes := raw.Get()
			if len(bytes) == 0 {
				return notice(chrome, message.Get(), "", nil)
			}
			return core.Image{
				Source: core.ImageSource{Data: bytes}, Label: label, ResizeMode: core.Contain,
				Style: core.Style{Width: core.Pct(100), Height: core.Pt(240)},
				OnError: func(error) {
					message.Set(w.tr("Unable to display this image", "无法显示该图片"))
					raw.Set(nil)
				},
			}
		})
	}
	return core.View{Style: core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), MinWidth: core.Pt(0), Background: chrome.panel},
		Children: []core.Node{bar(chrome, 28, chrome.elevated, chrome.label(label, chrome.muted, 12)), body}}
}
