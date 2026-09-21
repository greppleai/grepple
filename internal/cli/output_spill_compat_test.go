package cli

import "github.com/greppleai/grepple/internal/outputspill"

type spillOptions struct {
	disabled  bool
	threshold int
	directory string
}
type spilledOutputDescriptor = outputspill.Descriptor

func parseSpillOptions(args []string) ([]string, spillOptions, error) {
	filtered, options, err := outputspill.Parse(args)
	return filtered, spillOptions{disabled: options.Disabled, threshold: options.Threshold, directory: options.Directory}, err
}

func runWithOutputSpill(args []string, options spillOptions, run func() error) error {
	repository, _, err := loadRepositoryConfig()
	if err != nil {
		return err
	}
	return outputspill.Run(args, outputspill.Options{Disabled: options.disabled, Threshold: options.threshold, Directory: options.directory}, repository.Output.SpillThresholdBytes, mustGetwd(), func(int) error { return run() })
}
