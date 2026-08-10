package osiris

import (
	"context"
	"fmt"
	"path/filepath"
)

// lspTestResult is the JSON-safe IDE projection of LanguageTestResult. The
// internal error interface and evaluator values are deliberately omitted;
// clients receive stable status, output, assertion, log, and source data.
type lspTestResult struct {
	Name            string             `json:"name"`
	File            string             `json:"file"`
	Backend         string             `json:"backend"`
	Status          string             `json:"status"`
	Passed          bool               `json:"passed"`
	Skipped         bool               `json:"skipped,omitempty"`
	ExpectedFailure bool               `json:"expectedFailure,omitempty"`
	UnexpectedPass  bool               `json:"unexpectedPass,omitempty"`
	Outputs         []string           `json:"outputs,omitempty"`
	Assertions      []lspTestAssertion `json:"assertions,omitempty"`
	Logs            []string           `json:"logs,omitempty"`
	Location        lspLocation        `json:"location"`
	Error           string             `json:"error,omitempty"`
	DurationNS      int64              `json:"durationNS"`
}

type lspTestAssertion struct {
	Expression string      `json:"expression"`
	Passed     bool        `json:"passed"`
	Location   lspLocation `json:"location"`
	Message    string      `json:"message,omitempty"`
}

func (s *languageServer) testResultsContext(ctx context.Context, uri, backend, filter string) ([]lspTestResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, lspRequestCancelled(err)
	}
	if uri == "" {
		return nil, lspInvalidParams(fmt.Errorf("textDocument.uri is required"))
	}
	if backend != "" && backend != "interpreter" && backend != "vm" && backend != "native" {
		return nil, lspInvalidParams(fmt.Errorf("unsupported test backend %q", backend))
	}
	inventory, err := s.testInventoryContext(ctx, uri)
	if err != nil {
		return nil, err
	}
	overlays := s.languageTestOverlays(inventory.Files)
	results, err := RunLanguageTests(ctx, inventory.Files, LanguageTestOptions{
		Backend:        backend,
		Filter:         filter,
		Deterministic:  true,
		NoCache:        true,
		SourceOverlays: overlays,
	})
	if err != nil {
		return nil, err
	}
	out := make([]lspTestResult, 0, len(results))
	for _, result := range results {
		if err := ctx.Err(); err != nil {
			return nil, lspRequestCancelled(err)
		}
		out = append(out, lspTestResultFromResult(result))
	}
	return out, nil
}

func (s *languageServer) languageTestOverlays(files []string) map[string]string {
	allowed := make(map[string]struct{}, len(files)*2)
	for _, file := range files {
		clean := filepath.Clean(file)
		allowed[clean] = struct{}{}
		if absolute, err := filepath.Abs(clean); err == nil {
			allowed[filepath.Clean(absolute)] = struct{}{}
		}
		if source := colocatedSourceFile(clean); source != "" {
			allowed[filepath.Clean(source)] = struct{}{}
		}
	}
	overlays := make(map[string]string)
	for _, doc := range s.allDocuments() {
		if doc == nil {
			continue
		}
		path := filepath.Clean(doc.sourceName)
		if _, ok := allowed[path]; !ok {
			if absolute, err := filepath.Abs(path); err != nil {
				continue
			} else if _, ok := allowed[filepath.Clean(absolute)]; !ok {
				continue
			}
		}
		overlays[path] = doc.text
		if absolute, err := filepath.Abs(path); err == nil {
			overlays[filepath.Clean(absolute)] = doc.text
		}
	}
	return overlays
}

func lspTestResultFromResult(result LanguageTestResult) lspTestResult {
	out := lspTestResult{
		Name:            result.Name,
		File:            result.File,
		Backend:         result.Backend,
		Status:          result.Status,
		Passed:          result.Passed,
		Skipped:         result.Skipped,
		ExpectedFailure: result.ExpectedFailure,
		UnexpectedPass:  result.UnexpectedPass,
		Outputs:         append([]string(nil), result.OutputStrings...),
		Logs:            append([]string(nil), result.Logs...),
		Location:        lspTestLocation(result.Location),
		Error:           result.ErrorText,
		DurationNS:      result.DurationNS,
	}
	for _, assertion := range result.Assertions {
		out.Assertions = append(out.Assertions, lspTestAssertion{
			Expression: assertion.Expression,
			Passed:     assertion.Passed,
			Location:   lspTestLocation(assertion.Location),
			Message:    assertion.Message,
		})
	}
	return out
}

func lspTestLocation(location LanguageTestLocation) lspLocation {
	path := location.Source
	if path == "" {
		return lspLocation{}
	}
	return lspLocation{
		URI: pathToURI(path),
		Range: lspRange{
			Start: lspPosition{Line: maxInt(0, location.Start.Line-1), Character: maxInt(0, location.Start.Column-1)},
			End:   lspPosition{Line: maxInt(0, location.End.Line-1), Character: maxInt(0, location.End.Column-1)},
		},
	}
}
