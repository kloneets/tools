package notes

import (
	"strings"
	"testing"
)

const benchMarkdownDoc = `# Koko Tools

## Notes

- [ ] first task
- [x] second task
- [ ] third task

This is **bold**, *italic*, and ` + "`inline code`" + `.

> Quote line for markdown rendering.

1. one
2. two
3. three

` + "```go" + `
package main

import "fmt"

func main() {
	for i := 0; i < 10; i++ {
		fmt.Println("hello", i) // comment
	}
}
` + "```" + `

[OpenAI](https://openai.com)
`

const benchMarkdownMultiFenceDoc = `# Workspace

Regular paragraph with **bold**, _italic_, and ` + "`inline code`" + `.

` + "```go" + `
package main

import "fmt"

func main() {
    for i := 0; i < 8; i++ {
        fmt.Println("go", i)
    }
}
` + "```" + `

` + "```rust" + `
fn main() {
    for i in 0..8 {
        println!("rust {}", i);
    }
}
` + "```" + `

` + "```python" + `
def run():
    for i in range(8):
        print("py", i)
` + "```" + `

` + "```json" + `
{"name":"koko","enabled":true,"count":8}
` + "```" + `

` + "```bash" + `
for i in $(seq 1 8); do
  echo "$i"
done
` + "```" + `
`

func BenchmarkMarkdownSpans(b *testing.B) {
	text := strings.Repeat(benchMarkdownDoc+"\n", 20)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = markdownSpans(text)
	}
}

func BenchmarkMarkdownPreview(b *testing.B) {
	text := strings.Repeat(benchMarkdownDoc+"\n", 20)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = markdownPreview(text, 4)
	}
}

func BenchmarkMarkdownPreviewMultiFence(b *testing.B) {
	text := strings.Repeat(benchMarkdownMultiFenceDoc+"\n", 12)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = markdownPreview(text, 4)
	}
}

func BenchmarkTreeSitterSpansGo(b *testing.B) {
	text := strings.Repeat(`package main

import "fmt"

func main() {
	for i := 0; i < 10; i++ {
		fmt.Println("hello", i) // comment
	}
}
`, 30)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = treeSitterSpans(text, 0, "go")
	}
}

func BenchmarkTreeSitterSpansRust(b *testing.B) {
	text := strings.Repeat(`fn main() {
    for i in 0..10 {
        println!("hello {}", i);
    }
}
`, 30)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = treeSitterSpans(text, 0, "rust")
	}
}

func BenchmarkReplaceTextGlobal(b *testing.B) {
	text := strings.Repeat("alpha beta alpha gamma alpha\n", 200)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = replaceText(text, "alpha", "omega", true)
	}
}

func BenchmarkVimDeleteWord(b *testing.B) {
	text := strings.Repeat("alpha beta gamma delta ", 200)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = vimDeleteWord(text, 0)
	}
}

func BenchmarkVimPasteBlock(b *testing.B) {
	text := strings.Repeat("abcd efgh ijkl\n", 100)
	reg := vimRegister{Kind: vimRegisterBlock, Lines: []string{"XX", "YY", "ZZ"}}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = vimPasteBlock(text, 1, reg)
	}
}
