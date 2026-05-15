package emb

import (
	"testing"
	"time"
)

func TestProviderModelDimsGetters(t *testing.T) {
	f := NewFake()
	if f.Provider() != "ollama" || f.Model() != "nomic-embed-text" || f.Dims() != 8 {
		t.Fatalf("fake getters: %+v", f)
	}
	o := NewOllama("http://x", "m", time.Second, "30m")
	o.DimCount = 768
	if o.Provider() != "ollama" || o.Model() != "m" || o.Dims() != 768 {
		t.Fatalf("ollama getters")
	}
	oai := NewOpenAI("k", "m", time.Second)
	oai.DimCount = 1536
	if oai.Provider() != "openai" || oai.Model() != "m" || oai.Dims() != 1536 {
		t.Fatalf("openai getters")
	}
	v := NewVoyage("k", "m", time.Second)
	v.DimCount = 1024
	if v.Provider() != "voyage" || v.Model() != "m" || v.Dims() != 1024 {
		t.Fatalf("voyage getters")
	}
}
