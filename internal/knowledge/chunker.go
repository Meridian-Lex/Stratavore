package knowledge

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxChunkBytes  = 1500 // ~500 tokens, comfortable for granite-embedding context window
	minChunkBytes  = 50   // ignore trivially short sections
)

// ChunkFile reads a markdown file and splits it into Chunks by heading.
// If the file has no headings, the entire file is returned as a single chunk.
// Chunks shorter than minChunkBytes are skipped.
func ChunkFile(path string) ([]*Chunk, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	filename := filepath.Base(path)
	now := time.Now().UTC()

	var chunks []*Chunk
	var currentSection string
	var currentLines []string

	flush := func() {
		if len(currentLines) == 0 {
			return
		}
		content := strings.TrimSpace(strings.Join(currentLines, "\n"))
		if len(content) < minChunkBytes {
			return
		}
		// Split oversized sections into sub-chunks
		subChunks := splitBySize(content, maxChunkBytes)
		for i, sub := range subChunks {
			section := currentSection
			if len(subChunks) > 1 {
				section = fmt.Sprintf("%s (part %d/%d)", currentSection, i+1, len(subChunks))
			}
			hash := fmt.Sprintf("%x", sha256.Sum256([]byte(sub)))
			chunks = append(chunks, &Chunk{
				ID:          fmt.Sprintf("%s::%s::%s", filename, section, hash[:8]),
				SourceFile:  filename,
				Section:     section,
				Content:     sub,
				ContentHash: hash,
				UpdatedAt:   now,
			})
		}
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			flush()
			currentSection = strings.TrimSpace(strings.TrimLeft(line, "#"))
			currentLines = []string{}
		} else {
			currentLines = append(currentLines, line)
		}
	}
	flush() // flush final section

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", path, err)
	}

	// If no headings were found, treat the whole file as one chunk.
	if len(chunks) == 0 {
		content, _ := os.ReadFile(path)
		trimmed := strings.TrimSpace(string(content))
		if len(trimmed) >= minChunkBytes {
			hash := fmt.Sprintf("%x", sha256.Sum256(content))
			for i, sub := range splitBySize(trimmed, maxChunkBytes) {
				section := filename
				if i > 0 {
					section = fmt.Sprintf("%s (part %d)", filename, i+1)
				}
				subHash := fmt.Sprintf("%x", sha256.Sum256([]byte(sub)))
				chunks = append(chunks, &Chunk{
					ID:          fmt.Sprintf("%s::%s::%s", filename, section, hash[:8]),
					SourceFile:  filename,
					Section:     section,
					Content:     sub,
					ContentHash: subHash,
					UpdatedAt:   now,
				})
			}
		}
	}

	return chunks, nil
}

// splitBySize splits text into segments no larger than maxBytes,
// breaking at paragraph boundaries (double newline) where possible.
func splitBySize(text string, maxBytes int) []string {
	if len(text) <= maxBytes {
		return []string{text}
	}

	var parts []string
	paragraphs := strings.Split(text, "\n\n")
	var current strings.Builder

	for _, para := range paragraphs {
		// If the paragraph itself exceeds maxBytes, hard-split it.
		if len(para) > maxBytes {
			if current.Len() > 0 {
				parts = append(parts, strings.TrimSpace(current.String()))
				current.Reset()
			}
			for len(para) > maxBytes {
				parts = append(parts, strings.TrimSpace(para[:maxBytes]))
				para = para[maxBytes:]
			}
			if len(para) > 0 {
				current.WriteString(para)
			}
			continue
		}

		if current.Len()+len(para)+2 > maxBytes {
			if current.Len() > 0 {
				parts = append(parts, strings.TrimSpace(current.String()))
				current.Reset()
			}
		}
		if current.Len() > 0 {
			current.WriteString("\n\n")
		}
		current.WriteString(para)
	}

	if current.Len() > 0 {
		parts = append(parts, strings.TrimSpace(current.String()))
	}

	return parts
}
