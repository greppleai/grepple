package render

import "github.com/greppleai/grepple/api"

type filesRenderer struct {
	output *outputWriter
	json   bool
}

func (renderer filesRenderer) Render(results []api.FileResult) error {
	if renderer.json {
		return renderer.output.writeJSON(map[string]any{"files": filePathObjects(results)})
	}
	for _, result := range results {
		if err := renderer.output.writeString(result.Path + "\n"); err != nil {
			return err
		}
	}
	return nil
}
