package partners

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// docsDir is where the partner program documentation lives.
const docsDir = "../../docs/partners"

func readDoc(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(docsDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// TestAgreementCoversTheStory72Clauses guards AC2: the agreement must cover
// scope, retention, anonymization, feedback cadence and exit.
func TestAgreementCoversTheStory72Clauses(t *testing.T) {
	agreement := readDoc(t, "agreement.md")
	required := []string{
		"## 1. Escopo",
		"## 3. Tratamento de dados",
		"**Retenção**",
		"**Anonimização**",
		"## 4. Cadência de feedback",
		"## 7. Saída",
	}
	for _, anchor := range required {
		if !strings.Contains(agreement, anchor) {
			t.Errorf("agreement.md is missing the %q clause", anchor)
		}
	}
}

// TestAgreementMatchesTheOutreachSummary guards the risk called out in the
// Story 7.1 handoff: the outreach promise and the signed agreement must not
// diverge on retention or pilot shape.
func TestAgreementMatchesTheOutreachSummary(t *testing.T) {
	agreement := readDoc(t, "agreement.md")
	summary := readDoc(t, "data-handling.md")
	outreach := readDoc(t, "outreach.md")

	for _, pair := range []struct {
		value string
		docs  map[string]string
	}{
		{"7 dias", map[string]string{"agreement.md": agreement, "data-handling.md": summary, "outreach.md": outreach}},
		{"8 semanas", map[string]string{"agreement.md": agreement, "outreach.md": outreach}},
		{"30 min", map[string]string{"agreement.md": agreement, "outreach.md": outreach}},
	} {
		for name, body := range pair.docs {
			if !strings.Contains(body, pair.value) {
				t.Errorf("%s is missing %q; the agreement, the outreach and the summary must agree", name, pair.value)
			}
		}
	}
	if !strings.Contains(outreach, "7 days") {
		t.Error("outreach.md is missing the English retention value (7 days)")
	}
}

// TestSelectionDocMatchesTheValidator keeps the documented criteria aligned
// with the fields the validator actually enforces.
func TestSelectionDocMatchesTheValidator(t *testing.T) {
	selection := readDoc(t, "selection.md")
	for _, field := range []string{"uses_otel", "uses_kubernetes", "volume_events_per_second", "can_share_data", "can_weekly_feedback"} {
		if !strings.Contains(selection, field) {
			t.Errorf("selection.md does not mention the %q criterion", field)
		}
		if !containsColumn(field) {
			t.Errorf("%q is documented but not required by the validator", field)
		}
	}
	if !strings.Contains(selection, "10000") {
		t.Error("selection.md must state the 10k events/s threshold")
	}
}

func containsColumn(name string) bool {
	for _, column := range RequiredColumns {
		if column == name {
			return true
		}
	}
	return false
}

// TestBaselineTemplateCoversAC3 guards the baseline contract: tools, volume,
// cost and the top 3 pain points.
func TestBaselineTemplateCoversAC3(t *testing.T) {
	data, err := os.ReadFile("../../partners/baseline.template.md")
	if err != nil {
		t.Fatalf("read baseline template: %v", err)
	}
	template := string(data)
	for _, anchor := range []string{"## Ferramentas atuais", "## Volume", "## Custo", "## Top 3 dores"} {
		if !strings.Contains(template, anchor) {
			t.Errorf("baseline template is missing %q", anchor)
		}
	}
	if !strings.Contains(readDoc(t, "baseline.md"), "../../partners/baseline.template.md") {
		t.Error("baseline.md must point at the versioned template")
	}
	if !strings.Contains(readDoc(t, "agreement.md"), "baseline.md") {
		t.Error("agreement.md clause 5 must reference the baseline procedure")
	}
}

// TestPartnerDocsCarryNoContacts enforces the privacy rule: no e-mail address
// ever lands in the partner documentation.
func TestPartnerDocsCarryNoContacts(t *testing.T) {
	entries, err := os.ReadDir(docsDir)
	if err != nil {
		t.Fatalf("read %s: %v", docsDir, err)
	}
	email := regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		body := readDoc(t, entry.Name())
		if match := email.FindString(body); match != "" {
			t.Errorf("%s contains a contact address (%q); the tracker is the only place for PII", entry.Name(), match)
		}
	}
}
