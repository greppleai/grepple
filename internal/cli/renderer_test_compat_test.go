package cli

import (
	"github.com/greppleai/grepple/api"
	rendercommand "github.com/greppleai/grepple/internal/cli/render"
	"github.com/greppleai/grepple/search"
)

type segmentRenderer struct {
	output       *outputWriter
	anchors      anchorLookup
	contextGuard *segmentContextGuard
}

func (renderer segmentRenderer) Render(results []api.FileResult) error {
	_, err := rendercommand.Render(rendercommand.Options{JSON: "off", Anchors: rendercommand.AnchorLookup(renderer.anchors)}, results, renderer.output.writer, guardForRendererTest(renderer.contextGuard))
	return err
}

type jsonResultRenderer struct {
	output       *outputWriter
	contextGuard *segmentContextGuard
}

func (renderer jsonResultRenderer) Render(results []api.FileResult) error {
	_, err := rendercommand.Render(rendercommand.Options{JSON: "full"}, results, renderer.output.writer, guardForRendererTest(renderer.contextGuard))
	return err
}

type lineRenderer struct {
	output       *outputWriter
	anchors      anchorLookup
	contextGuard *segmentContextGuard
	repeatSource bool
}

func (renderer lineRenderer) Render(results []api.FileResult) error {
	_, err := rendercommand.Render(rendercommand.Options{JSON: "off", LineOnly: true, Anchors: rendercommand.AnchorLookup(renderer.anchors), RepeatSource: renderer.repeatSource}, results, renderer.output.writer, guardForRendererTest(renderer.contextGuard))
	return err
}

type contextRenderer struct {
	output       *outputWriter
	anchors      anchorLookup
	contextGuard *segmentContextGuard
}

func (renderer contextRenderer) Render(results []api.FileResult) error {
	_, err := rendercommand.Render(rendercommand.Options{JSON: "off", Params: search.Params{BeforeContext: 1}, Anchors: rendercommand.AnchorLookup(renderer.anchors)}, results, renderer.output.writer, guardForRendererTest(renderer.contextGuard))
	return err
}
func guardForRendererTest(guard *segmentContextGuard) rendercommand.ContextGuard {
	if guard == nil {
		return nil
	}
	return renderContextGuard{guard: guard}
}
