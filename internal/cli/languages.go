package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/extract"
	"github.com/greppleai/grepple/gritql"
	"github.com/greppleai/grepple/parser"
)

type languagesArgs struct {
	JSON     bool `arg:"--json" help:"print the complete capability matrix as JSON"`
	Markdown bool `arg:"--markdown" help:"print the generated documentation table"`
}

func (languagesArgs) Description() string {
	return "Show language extensions and support across search, navigation, focused extraction, GritQL, and directory architecture."
}

func runLanguages(args []string) error {
	values := languagesArgs{}
	argumentParser, err := arg.NewParser(arg.Config{Program: "grepple languages"}, &values)
	if err != nil {
		return err
	}
	if err := argumentParser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			argumentParser.WriteHelp(os.Stdout)
			return nil
		}
		return err
	}
	if values.JSON && values.Markdown {
		return fmt.Errorf("--json cannot be combined with --markdown")
	}
	capabilities := languageCapabilityMatrix()
	if values.JSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(capabilities)
	}
	if values.Markdown {
		_, err := fmt.Fprint(os.Stdout, renderLanguageCapabilitiesMarkdown(capabilities))
		return err
	}
	return renderLanguageCapabilities(capabilities)
}

func languageCapabilityMatrix() []api.LanguageCapabilities {
	extraction := extract.SupportedLanguages()
	gritLanguages := make(map[string]bool)
	for _, language := range gritql.SupportedLanguages() {
		gritLanguages[language.ID] = true
	}
	parserLanguages := make(map[string]parser.LanguageCapabilities)
	for _, language := range parser.SupportedLanguages() {
		parserLanguages[language.ID] = language
	}
	contentLanguages := parser.SupportedContentLanguages()
	result := make([]api.LanguageCapabilities, 0, len(contentLanguages))
	for _, language := range contentLanguages {
		extractLanguage, hasExtraction := extractionCapabilities(language, extraction)
		result = append(result, api.LanguageCapabilities{
			Language:              language.ID,
			Extensions:            append([]string{}, language.Extensions...),
			TextGrep:              api.FeatureProduction,
			StructuralGrep:        featureSupport(language.StructuralGrep, language.Specialized),
			Outline:               featureSupport(language.Outline, language.Specialized),
			Navigation:            featureSupport(language.Navigation, false),
			FocusedStructure:      featureSupport(hasExtraction && extractLanguage.FocusedStructure, false),
			FocusedFlow:           featureSupport(hasExtraction && extractLanguage.FocusedFlow, false),
			GritQL:                featureSupport(gritLanguages[language.ID], false),
			DirectoryArchitecture: featureSupport(language.Navigation, false),
			ImportRelations:       featureSupport(parserLanguages[language.ID].ImportNavigation, false),
			Entrypoints:           featureSupport(parserLanguages[language.ID].EntrypointNavigation, false),
			NavigationFacts:       navigationFactSupport(parserLanguages[language.ID].NavigationFacts),
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Language < result[j].Language })
	return result
}

func extractionCapabilities(language parser.ContentLanguageCapabilities, capabilities []extract.Language) (extract.Language, bool) {
	for _, capability := range capabilities {
		if capability.ID == language.ID || extensionsContained(language.Extensions, capability.Extensions) {
			return capability, true
		}
	}
	return extract.Language{}, false
}

func extensionsContained(required, available []string) bool {
	if len(required) == 0 {
		return false
	}
	set := make(map[string]bool, len(available))
	for _, extension := range available {
		set[extension] = true
	}
	for _, extension := range required {
		if !set[extension] {
			return false
		}
	}
	return true
}

func featureSupport(supported, specialized bool) api.FeatureSupport {
	if !supported {
		return api.FeatureUnsupported
	}
	if specialized {
		return api.FeatureSpecialized
	}
	return api.FeatureProduction
}

func navigationFactSupport(facts parser.NavigationFactCapabilities) api.NavigationFactCapabilities {
	return api.NavigationFactCapabilities{
		Declarations:   featureSupport(facts.Declarations, false),
		Calls:          featureSupport(facts.Calls, false),
		Imports:        featureSupport(facts.Imports, false),
		TypeReferences: featureSupport(facts.TypeReferences, false),
		Fields:         featureSupport(facts.Fields, false),
		MemberAccess:   featureSupport(facts.MemberAccess, false),
		Entrypoints:    featureSupport(facts.Entrypoints, false),
	}
}

func renderLanguageCapabilities(capabilities []api.LanguageCapabilities) error {
	writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "LANGUAGE\tEXTENSIONS\tTEXT\tSTRUCTURAL\tOUTLINE\tNAV\tSTRUCTURE\tFLOW\tGRITQL\tDIRECTORY\tIMPORT-RELATIONS\tENTRYPOINTS"); err != nil {
		return err
	}
	for _, capability := range capabilities {
		extensions := strings.Join(capability.Extensions, ",")
		if extensions == "" {
			extensions = "other"
		}
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			capability.Language, extensions, supportIcon(capability.TextGrep), supportIcon(capability.StructuralGrep),
			supportIcon(capability.Outline), supportIcon(capability.Navigation), supportIcon(capability.FocusedStructure),
			supportIcon(capability.FocusedFlow), supportIcon(capability.GritQL), supportIcon(capability.DirectoryArchitecture), supportIcon(capability.ImportRelations), supportIcon(capability.Entrypoints)); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(writer, "\nNAVIGATION FACT SUPPORT\nLANGUAGE\tDECLARATIONS\tCALLS\tIMPORTS\tTYPE-REFS\tFIELDS\tMEMBER-ACCESS\tENTRYPOINTS"); err != nil {
		return err
	}
	for _, capability := range capabilities {
		facts := capability.NavigationFacts
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", capability.Language,
			supportIcon(facts.Declarations), supportIcon(facts.Calls), supportIcon(facts.Imports), supportIcon(facts.TypeReferences),
			supportIcon(facts.Fields), supportIcon(facts.MemberAccess), supportIcon(facts.Entrypoints)); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(writer, "\n✓ production  ~ specialized production  - unsupported  ! experimental\nFact support means the adapter emits that fact kind; individual facts may remain ambiguous or unresolved."); err != nil {
		return err
	}
	return writer.Flush()
}

func supportIcon(support api.FeatureSupport) string {
	switch support {
	case api.FeatureProduction:
		return "✓"
	case api.FeatureSpecialized:
		return "~"
	case api.FeatureExperimental:
		return "!"
	default:
		return "-"
	}
}

func renderLanguageCapabilitiesMarkdown(capabilities []api.LanguageCapabilities) string {
	var output strings.Builder
	output.WriteString("| Language | Extensions | Text grep | Structural grep | Outline | Navigation | Focused structure | Focused flow | GritQL | Directory architecture | Import relations | Entrypoints |\n")
	output.WriteString("| --- | --- | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: |\n")
	for _, capability := range capabilities {
		extensions := "any other extension"
		if len(capability.Extensions) > 0 {
			extensions = "`" + strings.Join(capability.Extensions, "`, `") + "`"
		}
		fmt.Fprintf(&output, "| `%s` | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
			capability.Language, extensions, markdownSupportIcon(capability.TextGrep), markdownSupportIcon(capability.StructuralGrep),
			markdownSupportIcon(capability.Outline), markdownSupportIcon(capability.Navigation), markdownSupportIcon(capability.FocusedStructure),
			markdownSupportIcon(capability.FocusedFlow), markdownSupportIcon(capability.GritQL), markdownSupportIcon(capability.DirectoryArchitecture), markdownSupportIcon(capability.ImportRelations), markdownSupportIcon(capability.Entrypoints))
	}
	output.WriteString("\n### Navigation fact support\n\n")
	output.WriteString("| Language | Declarations | Calls | Imports | Type references | Fields | Member access | Entrypoints |\n")
	output.WriteString("| --- | :---: | :---: | :---: | :---: | :---: | :---: | :---: |\n")
	for _, capability := range capabilities {
		facts := capability.NavigationFacts
		fmt.Fprintf(&output, "| `%s` | %s | %s | %s | %s | %s | %s | %s |\n", capability.Language,
			markdownSupportIcon(facts.Declarations), markdownSupportIcon(facts.Calls), markdownSupportIcon(facts.Imports), markdownSupportIcon(facts.TypeReferences),
			markdownSupportIcon(facts.Fields), markdownSupportIcon(facts.MemberAccess), markdownSupportIcon(facts.Entrypoints))
	}
	return output.String()
}

func markdownSupportIcon(support api.FeatureSupport) string {
	switch support {
	case api.FeatureProduction:
		return "✅"
	case api.FeatureSpecialized:
		return "🟡"
	case api.FeatureExperimental:
		return "🧪"
	default:
		return "❌"
	}
}
