package contracts

import (
	"encoding/json"
	"errors"
	"io/fs"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/uuid"
)

// fullPayloads tem um payload de cada tipo com todos os campos preenchidos, inclusive os
// omitempty. Serializado num envelope, cada um precisa passar no schema do seu tipo; e
// TestFullPayloads_TodosOsCamposPreenchidos garante que um campo novo na struct entre aqui.
func fullPayloads() map[string]any {
	at := time.Date(2026, 11, 3, 12, 0, 0, 123456789, time.UTC)
	budgetID, paymentID := uuid.NewString(), uuid.NewString()
	requester := Requester{ID: uuid.NewString(), Name: "Ana Souza", Email: "ana@example.com", Document: "52998224725"}
	items := []Item{
		{Kind: ItemKindService, Ref: "4012", Description: "Troca de pastilhas", Quantity: 1, UnitPrice: 150},
		{Kind: ItemKindPart, Ref: "BOSCH-0986BB0606", Description: "Pastilha dianteira", Quantity: 2, UnitPrice: 90.5},
	}
	return map[string]any{
		CmdStartDiagnosis: StartDiagnosis{
			OSCode: 1042, Vehicle: Vehicle{Plate: "BRA2E19", Brand: "Fiat", Model: "Argo", Year: 2022},
			Notes: "barulho ao frear", SimulateFailure: FailBudget,
		},
		CmdStartRepair:     StartRepair{OSCode: 1042, Items: items, SimulateFailure: FailRepair},
		CmdCancelExecution: CancelExecution{Reason: "cancelada pela oficina"},
		CmdCreateBudget: CreateBudget{
			OSCode: 1042, Requester: requester, Items: items, Total: 331, SimulateFailure: FailBudget,
		},
		CmdRequestPayment:     RequestPayment{BudgetID: budgetID, SimulateFailure: FailPayment},
		CmdCancelBudget:       CancelBudget{BudgetID: budgetID, Reason: "cancelada pela oficina"},
		CmdRefundPayment:      RefundPayment{BudgetID: budgetID, Reason: "execução recusada"},
		EvtDiagnosisStarted:   DiagnosisStarted{StartedAt: at},
		EvtDiagnosisCompleted: DiagnosisCompleted{Services: []DiagnosedService{{Code: 4012}}, Parts: []DiagnosedPart{{ManufacturerCode: "BOSCH-0986BB0606", Quantity: 2}}, Notes: "pastilhas no limite"},
		EvtRepairStarted:      RepairStarted{StartedAt: at},
		EvtRepairFinished:     RepairFinished{FinishedAt: at},
		EvtRepairRejected:     RepairRejected{Reason: "capacidade esgotada"},
		EvtExecutionCancelled: ExecutionCancelled{Reason: "cancelada pela saga"},
		EvtBudgetCreated:      BudgetCreated{BudgetID: budgetID, Total: 331},
		EvtBudgetCreationFailed: BudgetCreationFailed{
			Reason: "total informado difere da soma dos itens",
		},
		EvtBudgetApproved:  BudgetApproved{BudgetID: budgetID, ApprovedAt: at},
		EvtBudgetRejected:  BudgetRejected{BudgetID: budgetID, Reason: "caro demais"},
		EvtBudgetCancelled: BudgetCancelled{BudgetID: budgetID, Reason: "cancelada pela oficina"},
		EvtPaymentLinkCreated: PaymentLinkCreated{
			BudgetID: budgetID, PaymentID: paymentID, CheckoutURL: "http://localhost:8082/fake-checkout/" + paymentID,
		},
		EvtPaymentApproved: PaymentApproved{
			BudgetID: budgetID, PaymentID: paymentID, ProviderPaymentID: "fake-" + paymentID, Amount: 331,
		},
		EvtPaymentFailed:   PaymentFailed{BudgetID: budgetID, PaymentID: paymentID, Reason: "recusado pelo provedor"},
		EvtPaymentRefunded: PaymentRefunded{BudgetID: budgetID, PaymentID: paymentID, Amount: 331},
		EvtServiceOrderStatusChanged: ServiceOrderStatusChanged{
			OSCode: 1042, Status: "awaiting_payment", PreviousStatus: "awaiting_approval", Requester: requester,
			TotalAmount: 331, CheckoutURL: "https://mp.test/checkout/1", Reason: "motivo",
		},
	}
}

func fullEnvelope(t *testing.T, msgType string, payload any) []byte {
	t.Helper()
	env, err := NewEnvelope(msgType, "test", Meta{
		SagaID: uuid.NewString(), OSID: uuid.NewString(), CorrelationID: "req-1", CausationID: uuid.NewString(),
	}, payload)
	if err != nil {
		t.Fatal(err)
	}
	body, err := env.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestSchemas_CompilamECobremTodosOsTipos(t *testing.T) {
	c, err := compileSchemas(schemaFiles)
	if err != nil {
		t.Fatalf("schemas não compilam: %v", err)
	}
	want := slices.Sorted(maps.Keys(fullPayloads()))
	if got := slices.Sorted(maps.Keys(c.byType)); !slices.Equal(got, want) {
		t.Fatalf("tipos com schema = %v\nconstantes do pacote = %v", got, want)
	}
	if got := MessageTypes(); !slices.Equal(got, want) {
		t.Fatalf("MessageTypes() = %v", got)
	}
	// Envelope e definições comuns compilam sozinhos (cada $def também).
	comp := newCompiler(schemaFiles)
	for _, loc := range []string{"envelope.schema.json", "common.schema.json"} {
		if _, err := comp.Compile(schemaBaseURL + loc); err != nil {
			t.Fatalf("%s: %v", loc, err)
		}
	}
}

func TestSchemas_VersaoDoEnvelope(t *testing.T) {
	docs := loadSchemaDocs(t)
	version := docs.at(t, "envelope.schema.json", "/properties/version")
	if got, ok := version["const"].(float64); !ok || int(got) != Version {
		t.Fatalf("envelope.schema.json fixa version=%v, contracts.Version=%d", version["const"], Version)
	}
}

func TestSchemas_ExemplosSaoValidos(t *testing.T) {
	for msgType := range fullPayloads() {
		t.Run(msgType, func(t *testing.T) {
			examples, err := Examples(msgType)
			if err != nil {
				t.Fatal(err)
			}
			if len(examples) == 0 {
				t.Fatal("schema sem examples")
			}
			for i, ex := range examples {
				if err := Validate(ex); err != nil {
					t.Fatalf("exemplo %d inválido: %v", i, err)
				}
				env, err := Parse(ex)
				if err != nil {
					t.Fatalf("exemplo %d não passa no Parse: %v", i, err)
				}
				if env.Type != msgType {
					t.Fatalf("exemplo %d é do tipo %q", i, env.Type)
				}
			}
		})
	}

	// Exemplos do envelope e de cada definição comum, contra o próprio schema.
	docs := loadSchemaDocs(t)
	comp := newCompiler(schemaFiles)
	check := func(loc string, node map[string]any) {
		t.Helper()
		examples, _ := node["examples"].([]any)
		if len(examples) == 0 {
			t.Fatalf("%s sem examples", loc)
		}
		sch, err := comp.Compile(schemaBaseURL + loc)
		if err != nil {
			t.Fatal(err)
		}
		for i, ex := range examples {
			if err := sch.Validate(ex); err != nil {
				t.Fatalf("%s: exemplo %d inválido: %v", loc, i, err)
			}
		}
	}
	check("envelope.schema.json", docs["envelope.schema.json"])
	for name, def := range docs.at(t, "common.schema.json", "/$defs") {
		if node := def.(map[string]any); node["type"] == "object" {
			check("common.schema.json#/$defs/"+name, node)
		}
	}
}

func TestFullPayloads_TodosOsCamposPreenchidos(t *testing.T) {
	for msgType, p := range fullPayloads() {
		assertFilled(t, msgType, reflect.ValueOf(p))
	}
}

func TestSchemas_StructsPreenchidasValidam(t *testing.T) {
	for msgType, p := range fullPayloads() {
		t.Run(msgType, func(t *testing.T) {
			if err := Validate(fullEnvelope(t, msgType, p)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestSchemas_EspelhamAsStructs compara a estrutura de cada schema com a struct Go: as mesmas
// propriedades que a struct serializa, obrigatórias exatamente as que não têm omitempty, e tipos
// JSON compatíveis (int → integer, float64 → number, time.Time → date-time, slice → array).
func TestSchemas_EspelhamAsStructs(t *testing.T) {
	docs := loadSchemaDocs(t)
	docs.compare(t, "Envelope", "envelope.schema.json", docs["envelope.schema.json"], reflect.TypeOf(Envelope{}))
	for msgType, p := range fullPayloads() {
		file := msgType + schemaSuffix
		doc := docs[file]
		if doc["title"] != msgType || doc["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
			t.Errorf("%s: title=%v $schema=%v", file, doc["title"], doc["$schema"])
		}
		if ref := docs.at(t, file, "/allOf/0")["$ref"]; ref != "envelope.schema.json" {
			t.Errorf("%s: allOf[0] deveria referenciar o envelope, veio %v", file, ref)
		}
		if c := docs.at(t, file, "/properties/type")["const"]; c != msgType {
			t.Errorf("%s: properties.type.const = %v", file, c)
		}
		docs.compare(t, msgType, file, docs.at(t, file, "/properties/payload"), reflect.TypeOf(p))
	}
}

func TestValidate_RecusaCampoObrigatorioAusente(t *testing.T) {
	docs := loadSchemaDocs(t)
	envRequired := stringsOf(docs["envelope.schema.json"]["required"])
	for msgType := range fullPayloads() {
		ex := firstExample(t, msgType)
		payloadRequired := stringsOf(docs.at(t, msgType+schemaSuffix, "/properties/payload")["required"])
		if len(payloadRequired) == 0 {
			t.Fatalf("%s: payload sem campos obrigatórios", msgType)
		}
		for _, field := range envRequired {
			body := mutate(t, ex, func(m map[string]any) { delete(m, field) })
			if err := Validate(body); err == nil {
				t.Errorf("%s sem %s foi aceito", msgType, field)
			}
		}
		for _, field := range payloadRequired {
			body := mutate(t, ex, func(m map[string]any) { delete(payloadOf(m), field) })
			if err := Validate(body); !errors.Is(err, ErrInvalidMessage) {
				t.Errorf("%s sem payload.%s: esperava ErrInvalidMessage, veio %v", msgType, field, err)
			}
		}
	}
}

func TestValidate_RecusaEnumFormatoETipoInvalidos(t *testing.T) {
	cases := []struct {
		name    string
		msgType string
		change  func(m map[string]any)
	}{
		{"status da OS desconhecido", EvtServiceOrderStatusChanged, func(m map[string]any) { payloadOf(m)["status"] = "paid" }},
		{"status anterior desconhecido", EvtServiceOrderStatusChanged, func(m map[string]any) { payloadOf(m)["previous_status"] = "x" }},
		{"kind do item desconhecido", CmdCreateBudget, func(m map[string]any) { itemOf(m)["kind"] = "labor" }},
		{"simulate_failure desconhecido no orçamento", CmdCreateBudget, func(m map[string]any) { payloadOf(m)["simulate_failure"] = "meteoro" }},
		{"simulate_failure desconhecido no pagamento", CmdRequestPayment, func(m map[string]any) { payloadOf(m)["simulate_failure"] = "all" }},
		{"simulate_failure desconhecido no reparo", CmdStartRepair, func(m map[string]any) { payloadOf(m)["simulate_failure"] = "Repair" }},
		{"simulate_failure desconhecido no diagnóstico", CmdStartDiagnosis, func(m map[string]any) { payloadOf(m)["simulate_failure"] = "x" }},
		{"message_id não é UUID", EvtBudgetCreated, func(m map[string]any) { m["message_id"] = "123" }},
		{"saga_id não é UUID", EvtBudgetCreated, func(m map[string]any) { m["saga_id"] = "saga-1" }},
		{"budget_id não é UUID", EvtBudgetCreated, func(m map[string]any) { payloadOf(m)["budget_id"] = "b-1" }},
		{"payment_id de PaymentFailed não é UUID nem vazio", EvtPaymentFailed, func(m map[string]any) { payloadOf(m)["payment_id"] = "p-1" }},
		{"occurred_at não é date-time", EvtBudgetCreated, func(m map[string]any) { m["occurred_at"] = "ontem" }},
		{"approved_at não é date-time", EvtBudgetApproved, func(m map[string]any) { payloadOf(m)["approved_at"] = "2026-13-01T00:00:00Z" }},
		{"checkout_url não é URI", EvtPaymentLinkCreated, func(m map[string]any) { payloadOf(m)["checkout_url"] = "mercadopago/checkout" }},
		{"os_code como texto", CmdCreateBudget, func(m map[string]any) { payloadOf(m)["os_code"] = "1042" }},
		{"os_code zero", CmdStartDiagnosis, func(m map[string]any) { payloadOf(m)["os_code"] = 0 }},
		{"quantidade zero", CmdStartRepair, func(m map[string]any) { itemOf(m)["quantity"] = 0 }},
		{"preço negativo", CmdCreateBudget, func(m map[string]any) { itemOf(m)["unit_price"] = -1 }},
		{"orçamento sem itens", CmdCreateBudget, func(m map[string]any) { payloadOf(m)["items"] = []any{} }},
		{"quantidade fracionária no diagnóstico", EvtDiagnosisCompleted, func(m map[string]any) {
			payloadOf(m)["parts"].([]any)[0].(map[string]any)["quantity"] = 1.5
		}},
		{"versão maior que a suportada", EvtBudgetCreated, func(m map[string]any) { m["version"] = Version + 1 }},
		{"payload que não é objeto", EvtBudgetCreated, func(m map[string]any) { m["payload"] = "x" }},
		{"requester que não é objeto", EvtServiceOrderStatusChanged, func(m map[string]any) { payloadOf(m)["requester"] = "Ana" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := mutate(t, firstExample(t, tc.msgType), tc.change)
			err := Validate(body)
			if !errors.Is(err, ErrInvalidMessage) {
				t.Fatalf("esperava ErrInvalidMessage, veio %v", err)
			}
			if !strings.Contains(err.Error(), tc.msgType) {
				t.Fatalf("o erro deveria citar o tipo: %v", err)
			}
		})
	}
}

func TestValidate_AceitaCamposDesconhecidosEOpcionaisVazios(t *testing.T) {
	extra := func(m map[string]any) {
		m["campo_novo"] = map[string]any{"qualquer": true}
		payloadOf(m)["campo_novo"] = []any{1, "dois"}
	}
	for msgType := range fullPayloads() {
		examples, err := Examples(msgType)
		if err != nil {
			t.Fatal(err)
		}
		for i, ex := range examples {
			if err := Validate(mutate(t, ex, extra)); err != nil {
				t.Errorf("%s exemplo %d com campo extra: %v", msgType, i, err)
			}
		}
	}
	nested := map[string]func(m map[string]any){
		CmdCreateBudget: func(m map[string]any) {
			itemOf(m)["desconto"] = 10
			payloadOf(m)["requester"].(map[string]any)["phone"] = "11999990000"
		},
		CmdStartDiagnosis: func(m map[string]any) { payloadOf(m)["vehicle"].(map[string]any)["color"] = "prata" },
		EvtDiagnosisCompleted: func(m map[string]any) {
			payloadOf(m)["services"].([]any)[0].(map[string]any)["name"] = "Troca de pastilhas"
		},
		// Campos opcionais e valores vazios que os produtores de fato geram.
		EvtPaymentFailed:  func(m map[string]any) { payloadOf(m)["payment_id"] = "" },
		CmdStartRepair:    func(m map[string]any) { payloadOf(m)["simulate_failure"] = FailRepair },
		CmdCancelBudget:   func(m map[string]any) { delete(m, "correlation_id"); delete(m, "causation_id") },
		EvtBudgetRejected: func(m map[string]any) { m["correlation_id"] = "timeout-" + uuid.NewString() },
		EvtDiagnosisStarted: func(m map[string]any) {
			payloadOf(m)["started_at"] = time.Date(2026, 11, 3, 9, 0, 0, 1, time.UTC).Format(time.RFC3339Nano)
		},
	}
	for msgType, change := range nested {
		if err := Validate(mutate(t, firstExample(t, msgType), change)); err != nil {
			t.Errorf("%s: %v", msgType, err)
		}
	}
}

func TestValidate_EntradaInvalida(t *testing.T) {
	cases := map[string]struct {
		body string
		want error
	}{
		"JSON quebrado":         {`{"type":`, ErrInvalidMessage},
		"lista no lugar":        {`[1,2]`, ErrInvalidMessage},
		"texto no lugar":        {`"billing.BudgetCreated"`, ErrInvalidMessage},
		"sem type":              {`{"message_id":"x"}`, ErrUnknownType},
		"type que não é string": {`{"type":42}`, ErrUnknownType},
		"type desconhecido":     {`{"type":"billing.BudgetExploded"}`, ErrUnknownType},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if err := Validate([]byte(tc.body)); !errors.Is(err, tc.want) {
				t.Fatalf("esperava %v, veio %v", tc.want, err)
			}
		})
	}
}

func TestExamples(t *testing.T) {
	if _, err := Examples("os.Unknown"); !errors.Is(err, ErrUnknownType) {
		t.Fatalf("esperava ErrUnknownType, veio %v", err)
	}
	a, err := Examples(EvtBudgetCreated)
	if err != nil {
		t.Fatal(err)
	}
	a[0][0] = 'X'
	b, _ := Examples(EvtBudgetCreated)
	if b[0][0] != '{' {
		t.Fatal("Examples deve devolver cópias")
	}
}

func TestCompileSchemas_Erros(t *testing.T) {
	valid := `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","examples":[{}]}`
	cases := map[string]fstest.MapFS{
		"sem schemas":          {},
		"JSON inválido":        {"schemas/x.Y.schema.json": {Data: []byte(`{`)}},
		"schema inválido":      {"schemas/x.Y.schema.json": {Data: []byte(`{"type":"objeto"}`)}},
		"ref inexistente":      {"schemas/x.Y.schema.json": {Data: []byte(`{"$ref":"nope.schema.json"}`)}},
		"ref para fora":        {"schemas/x.Y.schema.json": {Data: []byte(`{"$ref":"https://example.com/x.json"}`)}},
		"ref para subpasta":    {"schemas/x.Y.schema.json": {Data: []byte(`{"$ref":"sub/x.schema.json"}`)}},
		"examples não é lista": {"schemas/x.Y.schema.json": {Data: []byte(`{"examples":"x"}`)}},
	}
	for name, fsys := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := compileSchemas(fsys); err == nil {
				t.Fatal("esperava erro")
			}
		})
	}
	c, err := compileSchemas(fstest.MapFS{
		"schemas/x.Y.schema.json":      {Data: []byte(valid)},
		"schemas/envelope.schema.json": {Data: []byte(valid)},
	})
	if err != nil || len(c.byType) != 1 || len(c.examples["x.Y"]) != 1 {
		t.Fatalf("catálogo mínimo: %+v %v", c, err)
	}
}

// ---- utilitários ----

func firstExample(t *testing.T, msgType string) json.RawMessage {
	t.Helper()
	ex, err := Examples(msgType)
	if err != nil || len(ex) == 0 {
		t.Fatalf("%s sem exemplo: %v", msgType, err)
	}
	return ex[0]
}

// mutate aplica change numa cópia decodificada do exemplo e devolve o JSON resultante.
func mutate(t *testing.T, raw json.RawMessage, change func(m map[string]any)) []byte {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	change(m)
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func payloadOf(m map[string]any) map[string]any { return m["payload"].(map[string]any) }

func itemOf(m map[string]any) map[string]any {
	return payloadOf(m)["items"].([]any)[0].(map[string]any)
}

func stringsOf(v any) []string {
	list, _ := v.([]any)
	out := make([]string, 0, len(list))
	for _, s := range list {
		out = append(out, s.(string))
	}
	return out
}

var timeType = reflect.TypeOf(time.Time{})

// assertFilled falha se algum campo (recursivamente) estiver com o valor zero ou lista vazia.
func assertFilled(t *testing.T, where string, v reflect.Value) {
	t.Helper()
	switch {
	case v.Type() == timeType:
		if v.Interface().(time.Time).IsZero() {
			t.Errorf("%s está zerado", where)
		}
	case v.Kind() == reflect.Struct:
		for i := range v.NumField() {
			if f := v.Type().Field(i); f.IsExported() {
				assertFilled(t, where+"."+f.Name, v.Field(i))
			}
		}
	case v.Kind() == reflect.Slice:
		if v.Len() == 0 {
			t.Errorf("%s está vazio", where)
		}
		for i := range v.Len() {
			assertFilled(t, where+"["+strconv.Itoa(i)+"]", v.Index(i))
		}
	case v.IsZero():
		t.Errorf("%s está zerado", where)
	}
}

// schemaDocs são os schemas embutidos decodificados, por nome de arquivo.
type schemaDocs map[string]map[string]any

func loadSchemaDocs(t *testing.T) schemaDocs {
	t.Helper()
	names, err := fs.Glob(schemaFiles, schemaDir+"/*"+schemaSuffix)
	if err != nil {
		t.Fatal(err)
	}
	docs := schemaDocs{}
	for _, name := range names {
		raw, err := fs.ReadFile(schemaFiles, name)
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		docs[strings.TrimPrefix(name, schemaDir+"/")] = doc
	}
	return docs
}

// at devolve o nó do JSON Pointer ptr dentro do arquivo.
func (d schemaDocs) at(t *testing.T, file, ptr string) map[string]any {
	t.Helper()
	var node any = d[file]
	for _, tok := range strings.Split(strings.TrimPrefix(ptr, "/"), "/") {
		if tok == "" {
			continue
		}
		switch n := node.(type) {
		case map[string]any:
			node = n[tok]
		case []any:
			i, err := strconv.Atoi(tok)
			if err != nil || i >= len(n) {
				t.Fatalf("%s#%s: índice %q", file, ptr, tok)
			}
			node = n[i]
		}
	}
	m, ok := node.(map[string]any)
	if !ok {
		t.Fatalf("%s#%s não é um objeto", file, ptr)
	}
	return m
}

// resolve segue os $ref (relativos ao arquivo corrente) até um nó sem $ref.
func (d schemaDocs) resolve(t *testing.T, file string, node map[string]any) (string, map[string]any) {
	t.Helper()
	for range 10 {
		ref, ok := node["$ref"].(string)
		if !ok {
			return file, node
		}
		target, ptr, _ := strings.Cut(ref, "#")
		if target != "" {
			file = target
		}
		if _, ok := d[file]; !ok {
			t.Fatalf("$ref para arquivo inexistente: %s", ref)
		}
		node = d.at(t, file, ptr)
	}
	t.Fatalf("$ref em ciclo a partir de %s", file)
	return "", nil
}

type jsonField struct {
	typ       reflect.Type
	omitempty bool
}

func jsonFields(typ reflect.Type) map[string]jsonField {
	out := map[string]jsonField{}
	for i := range typ.NumField() {
		f := typ.Field(i)
		name, opts, _ := strings.Cut(f.Tag.Get("json"), ",")
		if !f.IsExported() || name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		out[name] = jsonField{typ: f.Type, omitempty: strings.Contains(opts, "omitempty")}
	}
	return out
}

func (d schemaDocs) compare(t *testing.T, where, file string, node map[string]any, typ reflect.Type) {
	t.Helper()
	file, node = d.resolve(t, file, node)
	wantType := func(jsonType string) {
		t.Helper()
		if node["type"] != jsonType {
			t.Errorf("%s: schema diz type=%v, a struct Go serializa %s (%s)", where, node["type"], jsonType, typ)
		}
	}
	switch {
	case typ == timeType:
		wantType("string")
		if node["format"] != "date-time" {
			t.Errorf("%s: time.Time precisa de format date-time, veio %v", where, node["format"])
		}
	case typ == reflect.TypeOf(json.RawMessage{}):
		wantType("object")
	case typ.Kind() == reflect.Struct:
		wantType("object")
		props, _ := node["properties"].(map[string]any)
		fields := jsonFields(typ)
		if got, want := slices.Sorted(maps.Keys(props)), slices.Sorted(maps.Keys(fields)); !slices.Equal(got, want) {
			t.Errorf("%s: propriedades do schema %v ≠ campos JSON da struct %v", where, got, want)
		}
		var required []string
		for name, f := range fields {
			if !f.omitempty {
				required = append(required, name)
			}
		}
		slices.Sort(required)
		if got := slices.Sorted(slices.Values(stringsOf(node["required"]))); !slices.Equal(got, required) {
			t.Errorf("%s: required do schema %v ≠ campos sem omitempty %v", where, got, required)
		}
		if node["additionalProperties"] != true {
			t.Errorf("%s: o schema deve aceitar campos desconhecidos (additionalProperties: true)", where)
		}
		for name, f := range fields {
			if p, ok := props[name].(map[string]any); ok {
				d.compare(t, where+"."+name, file, p, f.typ)
			}
		}
	case typ.Kind() == reflect.Slice:
		wantType("array")
		items, ok := node["items"].(map[string]any)
		if !ok {
			t.Errorf("%s: array sem items", where)
			return
		}
		d.compare(t, where+"[]", file, items, typ.Elem())
	case typ.Kind() == reflect.String:
		wantType("string")
	case typ.Kind() == reflect.Int:
		wantType("integer")
	case typ.Kind() == reflect.Float64:
		wantType("number")
	default:
		t.Errorf("%s: tipo Go sem mapeamento no teste: %s", where, typ)
	}
}
