package knowledge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---- chunker tests ----

func TestChunkFile_HeadingBased(t *testing.T) {
	content := `# Introduction

This is the introduction section with some content to ensure it meets the minimum byte threshold.

# Architecture

The architecture section describes the overall system design and components involved in the system.
`
	f := writeTempMD(t, content)
	chunks, err := ChunkFile(f)
	if err != nil {
		t.Fatalf("ChunkFile error: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if chunks[0].Section != "Introduction" {
		t.Errorf("expected section 'Introduction', got %q", chunks[0].Section)
	}
	if chunks[1].Section != "Architecture" {
		t.Errorf("expected section 'Architecture', got %q", chunks[1].Section)
	}
	// Chunk IDs must be unique
	if chunks[0].ID == chunks[1].ID {
		t.Error("chunk IDs must be unique")
	}
	// SourceFile must be the base filename
	for _, c := range chunks {
		if c.SourceFile != filepath.Base(f) {
			t.Errorf("expected SourceFile %q, got %q", filepath.Base(f), c.SourceFile)
		}
		if c.ContentHash == "" {
			t.Error("ContentHash must not be empty")
		}
	}
}

func TestChunkFile_NoHeadings(t *testing.T) {
	// A file with no headings and enough content to meet minChunkBytes.
	// The flush() at EOF captures content under an empty section ("").
	content := strings.Repeat("This is a sentence without a heading. ", 5)
	f := writeTempMD(t, content)
	chunks, err := ChunkFile(f)
	if err != nil {
		t.Fatalf("ChunkFile error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk for headingless file, got %d", len(chunks))
	}
	// Section is empty string when no heading precedes the content.
	if chunks[0].Section != "" {
		t.Errorf("section should be empty for headingless file, got %q", chunks[0].Section)
	}
	if chunks[0].SourceFile != filepath.Base(f) {
		t.Errorf("expected SourceFile %q, got %q", filepath.Base(f), chunks[0].SourceFile)
	}
}

func TestChunkFile_TooShortSkipped(t *testing.T) {
	// Content shorter than minChunkBytes per section — all chunks should be dropped.
	content := "# Short\n\nHi\n\n# AlsoShort\n\nBye\n"
	f := writeTempMD(t, content)
	chunks, err := ChunkFile(f)
	if err != nil {
		t.Fatalf("ChunkFile error: %v", err)
	}
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for short content, got %d", len(chunks))
	}
}

func TestChunkFile_CodeFenceNotHeading(t *testing.T) {
	// A '#' inside a fenced code block must not be treated as a heading.
	content := "# Real Heading\n\nSome intro text to meet the minimum byte threshold requirement.\n\n" +
		"```bash\n# this is a shell comment, not a heading\necho hello\n```\n\nMore content here.\n"
	f := writeTempMD(t, content)
	chunks, err := ChunkFile(f)
	if err != nil {
		t.Fatalf("ChunkFile error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk (code fence # ignored), got %d", len(chunks))
	}
	if chunks[0].Section != "Real Heading" {
		t.Errorf("expected section 'Real Heading', got %q", chunks[0].Section)
	}
}

func TestChunkFile_OversizedSectionSplit(t *testing.T) {
	// A section exceeding maxChunkBytes must be split into multiple parts.
	// Each paragraph is ~80 bytes; we need >1500 bytes total.
	var sb strings.Builder
	sb.WriteString("# Big Section\n\n")
	for i := 0; i < 25; i++ {
		sb.WriteString("This paragraph is a longer sentence designed to add bytes to the content.\n\n")
	}
	f := writeTempMD(t, sb.String())
	chunks, err := ChunkFile(f)
	if err != nil {
		t.Fatalf("ChunkFile error: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks for oversized section, got %d", len(chunks))
	}
	// All parts should have consistent part numbering
	for i, c := range chunks {
		expectedPart := i + 1
		if !strings.Contains(c.Section, "Big Section") {
			t.Errorf("chunk %d section should contain 'Big Section', got %q", i, c.Section)
		}
		if len(chunks) > 1 && !strings.Contains(c.Section, "(part") {
			t.Errorf("multi-part chunk %d should have '(part ...)', got %q", i, c.Section)
		}
		_ = expectedPart
	}
	// Chunk IDs must be distinct (different subHash)
	ids := make(map[string]bool)
	for _, c := range chunks {
		if ids[c.ID] {
			t.Errorf("duplicate chunk ID: %q", c.ID)
		}
		ids[c.ID] = true
	}
}

func TestChunkFile_NotFound(t *testing.T) {
	_, err := ChunkFile("/nonexistent/path/to/file.md")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

// ---- splitBySize tests ----

func TestSplitBySize_SingleChunk(t *testing.T) {
	text := strings.Repeat("x", 100)
	parts := splitBySize(text, 200)
	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(parts))
	}
	if parts[0] != text {
		t.Error("single part should equal input")
	}
}

func TestSplitBySize_ParagraphBoundary(t *testing.T) {
	para := strings.Repeat("word ", 30) // ~150 bytes per paragraph
	text := para + "\n\n" + para + "\n\n" + para
	parts := splitBySize(text, 200)
	if len(parts) < 2 {
		t.Fatalf("expected multiple parts, got %d", len(parts))
	}
	for _, p := range parts {
		if len(p) > 200 {
			t.Errorf("part exceeds max size: %d", len(p))
		}
	}
}

func TestSplitBySize_VeryLongParagraph(t *testing.T) {
	// A single paragraph larger than maxBytes should be hard-split.
	text := strings.Repeat("z", 3000)
	parts := splitBySize(text, 1000)
	for _, p := range parts {
		if len(p) > 1000 {
			t.Errorf("hard-split part exceeds max: %d", len(p))
		}
	}
}

// ---- cache key determinism ----

func TestCacheKey_Deterministic(t *testing.T) {
	// cacheKey is an internal method; exercise via two cache instances.
	c1 := &KnowledgeCache{}
	c2 := &KnowledgeCache{}
	k1 := c1.cacheKey("find rank promotion rules", 5)
	k2 := c2.cacheKey("find rank promotion rules", 5)
	if k1 != k2 {
		t.Errorf("cache key not deterministic: %q vs %q", k1, k2)
	}
}

func TestCacheKey_DifferentInputsDifferentKeys(t *testing.T) {
	c := &KnowledgeCache{}
	k1 := c.cacheKey("query one", 5)
	k2 := c.cacheKey("query two", 5)
	k3 := c.cacheKey("query one", 10)
	if k1 == k2 {
		t.Error("different queries should produce different keys")
	}
	if k1 == k3 {
		t.Error("different k values should produce different keys")
	}
	if !strings.HasPrefix(k1, CacheKeyPrefix) {
		t.Errorf("cache key should have prefix %q, got %q", CacheKeyPrefix, k1)
	}
}

// ---- helpers ----

func writeTempMD(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "*.md")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	f.Close()
	return f.Name()
}
