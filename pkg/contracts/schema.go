package contracts

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Os JSON Schemas (draft 2020-12) de cada tipo de mensagem, do envelope (envelope.schema.json) e
// dos tipos compartilhados (common.schema.json). O schema de um tipo descreve o envelope inteiro,
// com o payload daquele tipo, e traz em "examples" mensagens completas, prontas para os testes de
// contrato dos consumidores. O AsyncAPI do os-service referencia estes mesmos arquivos.
//
//go:embed schemas/*.schema.json
var schemaFiles embed.FS

const (
	schemaDir     = "schemas"
	schemaSuffix  = ".schema.json"
	schemaBaseURL = "https://github.com/Kc1t/postech-tc4-os-service/raw/main/pkg/contracts/schemas/"
)

// Schemas que não são tipos de mensagem.
const (
	envelopeSchema = "envelope"
	commonSchema   = "common"
)

var (
	// ErrInvalidMessage indica uma mensagem que não segue o contrato do seu tipo.
	ErrInvalidMessage = errors.New("contracts: mensagem fora do contrato")
	// ErrUnknownType indica um tipo de mensagem sem schema publicado.
	ErrUnknownType = errors.New("contracts: tipo de mensagem sem schema")
)

type catalog struct {
	byType   map[string]*jsonschema.Schema
	examples map[string][]json.RawMessage
}

var (
	loadOnce   sync.Once
	loaded     *catalog
	errCatalog error
)

func defaultCatalog() (*catalog, error) {
	loadOnce.Do(func() { loaded, errCatalog = compileSchemas(schemaFiles) })
	return loaded, errCatalog
}

// Validate valida o corpo serializado de uma mensagem (o JSON do envelope) contra o JSON Schema
// do seu tipo: campos obrigatórios, tipos, enums e formatos do envelope e do payload. Campos
// desconhecidos são aceitos (leitor tolerante). Devolve ErrUnknownType para tipo sem schema e
// ErrInvalidMessage, com o detalhe da violação, para mensagem fora do contrato.
func Validate(raw []byte) error {
	c, err := defaultCatalog()
	if err != nil {
		return err
	}
	return c.validate(raw)
}

// Examples devolve os exemplos do schema do tipo: envelopes completos, válidos contra o contrato.
func Examples(msgType string) ([]json.RawMessage, error) {
	c, err := defaultCatalog()
	if err != nil {
		return nil, err
	}
	ex, ok := c.examples[msgType]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownType, msgType)
	}
	out := make([]json.RawMessage, len(ex))
	for i, e := range ex {
		out[i] = slices.Clone(e)
	}
	return out, nil
}

// MessageTypes lista, em ordem alfabética, os tipos de mensagem com schema publicado.
func MessageTypes() []string {
	c, err := defaultCatalog()
	if err != nil {
		return nil
	}
	types := make([]string, 0, len(c.byType))
	for t := range c.byType {
		types = append(types, t)
	}
	slices.Sort(types)
	return types
}

func (c *catalog) validate(raw []byte) error {
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("%w: JSON inválido: %w", ErrInvalidMessage, err)
	}
	obj, ok := inst.(map[string]any)
	if !ok {
		return fmt.Errorf("%w: o envelope precisa ser um objeto JSON", ErrInvalidMessage)
	}
	msgType, _ := obj["type"].(string)
	sch, ok := c.byType[msgType]
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownType, msgType)
	}
	if err := sch.Validate(inst); err != nil {
		return fmt.Errorf("%w: %s: %w", ErrInvalidMessage, msgType, err)
	}
	return nil
}

// compileSchemas compila todos os schemas de fsys/schemas. As referências entre arquivos
// ($ref relativo) só são resolvidas dentro de fsys: nada é lido do disco nem da rede.
func compileSchemas(fsys fs.FS) (*catalog, error) {
	names, err := fs.Glob(fsys, schemaDir+"/*"+schemaSuffix)
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, errors.New("contracts: nenhum schema embutido")
	}
	comp := newCompiler(fsys)
	c := &catalog{byType: map[string]*jsonschema.Schema{}, examples: map[string][]json.RawMessage{}}
	for _, name := range names {
		base := path.Base(name)
		sch, err := comp.Compile(schemaBaseURL + base)
		if err != nil {
			return nil, fmt.Errorf("contracts: compilar %s: %w", base, err)
		}
		msgType := strings.TrimSuffix(base, schemaSuffix)
		if msgType == envelopeSchema || msgType == commonSchema {
			continue
		}
		raw, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		var doc struct {
			Examples []json.RawMessage `json:"examples"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, fmt.Errorf("contracts: ler exemplos de %s: %w", base, err)
		}
		c.byType[msgType] = sch
		c.examples[msgType] = doc.Examples
	}
	return c, nil
}

// newCompiler cria o compilador: draft 2020-12 com formatos (uuid, date-time, uri) verificados,
// que no 2020-12 seriam só anotação.
func newCompiler(fsys fs.FS) *jsonschema.Compiler {
	comp := jsonschema.NewCompiler()
	comp.DefaultDraft(jsonschema.Draft2020)
	comp.AssertFormat()
	comp.UseLoader(fsLoader{fsys: fsys})
	return comp
}

// fsLoader entrega ao compilador os schemas embutidos, endereçados por schemaBaseURL.
type fsLoader struct{ fsys fs.FS }

func (l fsLoader) Load(url string) (any, error) {
	name, ok := strings.CutPrefix(url, schemaBaseURL)
	if !ok || strings.Contains(name, "/") {
		return nil, fmt.Errorf("contracts: schema fora do pacote: %s", url)
	}
	raw, err := fs.ReadFile(l.fsys, schemaDir+"/"+name)
	if err != nil {
		return nil, err
	}
	return jsonschema.UnmarshalJSON(bytes.NewReader(raw))
}
