# Capability Router v2 — Design & Implementation Plan

> harness-downshift by Tiago de Carvalho Vilas Boas
> https://github.com/tiagovilasboas/harness-downshift

---

## 1. Contexto e Motivação

### O que temos hoje (v1)

Um classificador determinístico de uma dimensão:

```
prompt → regex scoring → complexidade (Trivial/Simple/Medium/Complex) → tier → modelo mais barato do tier
```

**Pontos fortes:**
- Zero LLM no loop, zero token overhead
- Single binary, fail-open, auditável
- Agnóstico de provider (catalog separado da lógica)

**Limites estruturais:**
- Complexidade e capacidade são tratadas como sinônimos
- Segurança está diluída em pesos de regex
- Tuning = edição manual de regex, sem feedback loop
- Não há consciência de risco (errar pra baixo custa igual a errar pra cima)

### O que a v2 resolve

| Limite v1 | Solução v2 |
|-----------|------------|
| Complexidade = capacidade | Feature extraction com 13 sinais + capability requirements separados |
| Segurança diluída | Safety rules como piso determinístico, fora do estatístico |
| Sem consciência de risco | Risk-weighted loss + policy que penaliza under-routing |
| Tuning manual | `downshift train <dataset>` offline com métricas antes de ativar |
| Modelo fixo por tier | ModelProfile com capabilities, matcher por custo + requisitos |

---

## 2. Princípios de Design

### 2.1 Isolamento e Agnóstico de Modelo

O router v2 é **completamente isolado** do conhecimento de modelos específicos:

- Nenhum nome de modelo (Claude, GPT, Grok) aparece no código do router
- Nenhum provider (Anthropic, OpenAI, xAI) aparece no código do router
- Decisões são baseadas em **tiers + capabilities + custo**, nunca em strings
- ModelProfile vem do catalog, não é hardcoded

### 2.2 Algoritmo Plugável

O classifier é uma interface, não uma implementação fixa:

```go
type Classifier interface {
    Classify(features FeatureVector) (TierProbabilities, error)
}
```

**Hoje:** `SoftmaxClassifier` (logística linear + softmax)
**Amanhã:** XGBoost, ensemble, LLM-based — sem mudar o resto do pipeline

### 2.3 KISS / YAGNI / SRP

- Cada pacote tem uma única responsabilidade
- Abstrações só onde pagam seu custo (interface Classifier sim, framework de plugins não)
- Features especulativas ficam documentadas, não implementadas

### 2.4 Fail-Open

- Se weights.json faltar → rota conservadora determinística
- Se extractor falhar → fallback pro legacy
- Se classifier der erro → safety rules ainda aplicam
- Nenhum erro bloqueia o spawn do subagente

---

## 3. Arquitetura

### 3.1 Estrutura de Pacotes

```
internal/routingv2/
  domain/
    types.go         → FeatureVector, CapabilityReq, SafetyConstraint,
                       TierProbabilities, RoutingInput, RoutingDecision
    profile.go       → ModelProfile (capabilities do catalog)
    router.go        → Router interface
    classifier.go    → Classifier interface (plugável)

  extractor/
    extractor.go     → task → FeatureVector
    lexical.go       → sinais léxicos (keywords, patterns)
    structural.go    → sinais estruturais (tamanho, formatação)

  safety/
    safety.go        → FeatureVector → SafetyConstraint
    rules.go         → regras determinísticas (auth, migration, concurrency...)

  classifier/
    softmax.go       → SoftmaxClassifier implements Classifier
    weights.go       → carrega weights.json (embedded + override)
    loss.go          → risk-weighted loss (usado no training)

  policy/
    policy.go        → probabilities + confidence + safety → tier alvo
    risk.go          → lógica de penalização assimétrica

  matcher/
    matcher.go       → tier + requirements + profiles → modelo mais barato
    profile.go       → extrai ModelProfile do catalog

  training/
    train.go         → CLI offline, gera weights.json
    dataset.go       → parsing de dataset rotulado
    metrics.go       → accuracy, unsafe downgrade, risk loss, confusion matrix

  router/
    legacy.go        → LegacyRouter (wrap de core.Route)
    capability.go    → CapabilityRouter (pipeline completo)
    config.go        → escolha de router por config/env

internal/routingv2/weights/
  default.json       → embedded via go:embed
```

### 3.2 Direção de Dependências

```
routingv2/domain    → (nenhuma dependência interna)
routingv2/extractor → domain
routingv2/safety    → domain
routingv2/classifier→ domain
routingv2/policy    → domain
routingv2/matcher   → domain, core.Resolver (interface, não catalog)
routingv2/training  → domain, extractor, classifier (offline only)
routingv2/router    → domain, extractor, safety, classifier, policy, matcher

core                → (não importa routingv2)
catalog             → core (não importa routingv2)
adapters            → core, catalog, routingv2/router (opcional, por config)
```

### 3.3 Interface Router

```go
type Router interface {
    Route(ctx context.Context, input RoutingInput) (RoutingDecision, error)
}

type RoutingInput struct {
    Prompt         string
    Harness        string
    CurrentModelID string
    Resolver       core.Resolver
}

type RoutingDecision struct {
    Tier           core.Tier
    Effort         core.Effort
    Model          core.Model
    CurrentModel   core.Model
    Verdict        core.Verdict
    Savings        float64
    Confidence     float64
    Features       FeatureVector      // para telemetria/debug
    Safety         SafetyConstraint   // para auditoria
    Probabilities  TierProbabilities  // para debug
}
```

### 3.4 Pipeline do CapabilityRouter

```
RoutingInput
    │
    ▼
┌─────────────┐
│  Extractor  │ → FeatureVector (13 sinais normalizados 0..1)
└─────────────┘
    │
    ▼
┌─────────────┐
│   Safety    │ → SafetyConstraint (min tier, nunca escolhe model)
└─────────────┘
    │
    ▼
┌─────────────┐
│ Classifier  │ → TierProbabilities + Confidence
└─────────────┘
    │
    ▼
┌─────────────┐
│   Policy    │ → Tier alvo (risk-aware, respeita safety)
└─────────────┘
    │
    ▼
┌─────────────┐
│   Matcher   │ → Model (menor custo que atende tier + capabilities)
└─────────────┘
    │
    ▼
RoutingDecision
```

---

## 4. Features e Capabilities

### 4.1 Feature Vector (13 sinais)

| Sinal | Descrição | Range |
|-------|-----------|-------|
| `mechanical` | Tarefa mecânica (rename, format, lint) | 0..1 |
| `coding` | Produção de código | 0..1 |
| `debugging` | Investigação de bug | 0..1 |
| `refactoring` | Reestruturação de código | 0..1 |
| `architecture` | Design de sistema | 0..1 |
| `migration` | Migração de dados/schema | 0..1 |
| `security` | Auth, crypto, access control | 0..1 |
| `concurrency` | Race conditions, deadlocks | 0..1 |
| `planning` | Planejamento multi-step | 0..1 |
| `tool_use` | Uso de ferramentas externas | 0..1 |
| `ambiguity` | Requisitos vagos ou conflitantes | 0..1 |
| `cross_module` | Mudança em múltiplos módulos | 0..1 |
| `context_size` | Estimativa de contexto necessário | 0..1 |

### 4.2 Capability Requirements (7 dimensões)

| Capability | Descrição |
|------------|-----------|
| `coding` | Capacidade de produzir código correto |
| `reasoning` | Capacidade de raciocínio lógico profundo |
| `planning` | Capacidade de decomposição multi-step |
| `debugging` | Capacidade de investigação e diagnóstico |
| `security` | Conhecimento de práticas seguras |
| `tool_use` | Capacidade de usar ferramentas corretamente |
| `long_context` | Capacidade de manter coerência em contexto longo |

### 4.3 Safety Rules (piso determinístico)

| Trigger | Min Tier | Razão |
|---------|----------|-------|
| `security > 0.7` | FRONTIER | Auth/crypto não pode errar |
| `migration > 0.6` | FRONTIER | Dados em risco |
| `concurrency > 0.6` | FRONTIER | Race conditions são sutis |
| `architecture > 0.7 AND cross_module > 0.5` | FRONTIER | Blast radius alto |
| `ambiguity > 0.8` | MID (min) | Precisa de clarificação |

Safety **nunca** escolhe model ID. Só impõe restrição de tier.

---

## 5. Algoritmo de Classificação

### 5.1 Softmax Classifier (default)

Regressão logística multinomial:

```
scores[tier] = dot(weights[tier], features) + bias[tier]
probabilities = softmax(scores)
confidence = max(probabilities) - second_max(probabilities)
```

### 5.2 Risk-Weighted Loss (treinamento)

```
loss = Σ cross_entropy(y, ŷ) * risk_weight(y_true, y_pred)

risk_weight:
  FRONTIER → SMALL: 10.0  (erro catastrófico)
  FRONTIER → MID:    5.0  (erro caro)
  MID → SMALL:       2.0  (erro moderado)
  SMALL → MID:       0.5  (over-routing barato)
  SMALL → FRONTIER:  1.0  (over-routing, mas seguro)
  correto:           1.0
```

### 5.3 Policy Risk-Aware

```
if confidence < 0.4:
    tier = max(classifier_tier, safety_tier, MID)  // conservador
else:
    tier = max(classifier_tier, safety_tier)
```

---

## 6. Model Matcher

### 6.1 ModelProfile no Catalog

Novo campo opcional no `catalog.json`:

```json
{
  "id": "claude-sonnet-4",
  "harness": "claude-code",
  "tier": "mid",
  "capabilities": {
    "coding": 0.8,
    "reasoning": 0.7,
    "planning": 0.6,
    "debugging": 0.7,
    "security": 0.6,
    "tool_use": 0.8,
    "long_context": 0.7
  },
  "input_cost_per_1m": 3.0,
  "output_cost_per_1m": 15.0
}
```

Quando `capabilities` está ausente, deriva default do tier:

| Tier | Default Capabilities |
|------|---------------------|
| SMALL | all: 0.4 |
| MID | all: 0.7 |
| FRONTIER | all: 0.95 |

### 6.2 Algoritmo do Matcher

```
1. Filtrar modelos do harness
2. Filtrar por tier >= tier_alvo
3. Filtrar por capabilities >= requirements
4. Excluir explicit_only
5. Ordenar por custo (input + output)
6. Retornar o mais barato
7. Se nenhum passar, retornar o mais barato do tier (fail-open)
```

---

## 7. Training Offline

### 7.1 Comando

```bash
downshift train <dataset.json> [--output weights.json] [--validation-split 0.2]
```

### 7.2 Dataset Format

```json
[
  {
    "prompt": "rename the variable userId",
    "label": "SMALL",
    "features": {}  // opcional, se ausente extrai automaticamente
  },
  {
    "prompt": "rearchitect the payment module",
    "label": "FRONTIER"
  }
]
```

Labels: `SMALL`, `MID`, `FRONTIER` (tier, não complexity)

### 7.3 Output

```
Training capability-router v2

Dataset:        120 tasks
Train/Val:      96 / 24

Epoch 100/100
  Train loss:     0.342
  Train accuracy: 87.5%

Validation:
  Tier accuracy:      83.3%
  Unsafe downgrade:    4.2%  (1/24)
    FRONTIER → MID:    4.2%  (1/24)
    FRONTIER → SMALL:  0.0%  (0/24)
  Risk-weighted loss: 0.089

Confusion Matrix (validation):
             SMALL   MID  FRONT  (predicted)
Actual SMALL     6     2     0
Actual MID       1     8     1
Actual FRONTIER  0     1     5

Weights saved to: weights.json
Version: 2024-01-15T10:30:00Z
```

---

## 8. Benchmark Comparativo

### 8.1 Comando

```bash
downshift benchmark <dataset.json> --compare
```

### 8.2 Output

```
Benchmark: Legacy vs Capability Router v2

Dataset: 120 tasks

                        Legacy    Capability v2    Delta
Tier accuracy           78.3%         85.0%       +6.7%
Unsafe downgrade        12.5%          4.2%       -8.3%  ✓
  FRONTIER → MID         8.3%          4.2%       -4.1%  ✓
  FRONTIER → SMALL       4.2%          0.0%       -4.2%  ✓
Wasteful over-routing   10.0%         14.2%       +4.2%
Risk-weighted loss      0.182         0.089       -51%   ✓
Avg confidence            —           0.72          —

Recommendation: Capability v2 reduces unsafe downgrades significantly.
                Accept slightly higher over-routing for safety gain.
```

---

## 9. Tasks de Implementação

### Fase 1: Fundação (este PR)

- [ ] **1.1** Criar `internal/routingv2/domain/` com tipos base
  - `types.go`: FeatureVector, CapabilityReq, SafetyConstraint, TierProbabilities
  - `profile.go`: ModelProfile
  - `router.go`: Router interface
  - `classifier.go`: Classifier interface

- [ ] **1.2** Implementar `internal/routingv2/extractor/`
  - `extractor.go`: Extract(prompt) → FeatureVector
  - `lexical.go`: sinais baseados em keywords/patterns
  - `structural.go`: sinais baseados em tamanho/formato

- [ ] **1.3** Implementar `internal/routingv2/safety/`
  - `rules.go`: regras determinísticas
  - `safety.go`: Evaluate(features) → SafetyConstraint

- [ ] **1.4** Implementar `internal/routingv2/classifier/`
  - `softmax.go`: SoftmaxClassifier
  - `weights.go`: LoadWeights (embedded + override)
  - `loss.go`: RiskWeightedLoss (para training)

- [ ] **1.5** Implementar `internal/routingv2/policy/`
  - `policy.go`: Decide(probs, confidence, safety) → Tier
  - `risk.go`: lógica de penalização

- [ ] **1.6** Implementar `internal/routingv2/matcher/`
  - `matcher.go`: Match(tier, requirements, resolver) → Model
  - `profile.go`: ProfileFromEntry(catalog.Entry) → ModelProfile

- [ ] **1.7** Implementar `internal/routingv2/router/`
  - `legacy.go`: LegacyRouter wraps core.Route
  - `capability.go`: CapabilityRouter pipeline completo
  - `config.go`: NewRouter(config) → Router

- [ ] **1.8** Adicionar campo `capabilities` opcional no catalog
  - Atualizar `internal/catalog/catalog.go`
  - Atualizar `catalog.sample.json`
  - Manter retrocompat (default por tier)

- [ ] **1.9** Criar `internal/routingv2/weights/default.json`
  - Pesos iniciais razoáveis (podem ser ajustados com train)

- [ ] **1.10** Implementar `internal/routingv2/training/`
  - `train.go`: Train(dataset) → weights
  - `dataset.go`: LoadDataset
  - `metrics.go`: Accuracy, UnsafeDowngrade, RiskLoss, ConfusionMatrix

- [ ] **1.11** Adicionar `downshift train` no cmd
  - Parsing de args
  - Output formatado
  - Salvar weights.json

- [ ] **1.12** Estender `downshift benchmark` para comparar routers
  - Flag `--compare`
  - Métricas lado a lado
  - Recomendação baseada em unsafe downgrade

- [ ] **1.13** Testes unitários
  - extractor_test.go
  - safety_test.go
  - softmax_test.go
  - policy_test.go
  - matcher_test.go
  - capability_router_test.go
  - legacy_router_test.go

- [ ] **1.14** Rodar `go test -race ./...`

- [ ] **1.15** Rodar benchmark comparativo e documentar resultados

### Fase 2: Validação (pós-merge)

- [ ] **2.1** Shadow mode nos adapters (log v2, usa legacy)
- [ ] **2.2** Coletar telemetria real por 1-2 semanas
- [ ] **2.3** Retreinar com dados reais
- [ ] **2.4** Quando métricas forem melhores, trocar default

### Fase 3: Evolução (futuro, não agora)

- [ ] **3.1** Exportar dataset de telemetria para treino por outcome
- [ ] **3.2** Considerar repository/harness profiles
- [ ] **3.3** Considerar algoritmo mais sofisticado (XGBoost, ensemble)
- [ ] **3.4** Considerar LLM-based classifier para casos especiais

---

## 10. Preparado para Escala (não implementado agora)

Decisões de design que facilitam evolução futura:

### 10.1 Algoritmo Plugável

A interface `Classifier` permite trocar o algoritmo sem mudar o pipeline:

```go
// Hoje
router := NewCapabilityRouter(NewSoftmaxClassifier(weights))

// Amanhã (XGBoost)
router := NewCapabilityRouter(NewXGBoostClassifier(modelPath))

// Amanhã (LLM-based)
router := NewCapabilityRouter(NewLLMClassifier(client, prompt))

// Amanhã (Ensemble)
router := NewCapabilityRouter(NewEnsembleClassifier(
    NewSoftmaxClassifier(weights),
    NewXGBoostClassifier(modelPath),
))
```

### 10.2 Multi-Profile Weights

O design atual usa um `weights.json` único. Para profiles por contexto:

```go
// Futuro
type WeightSource interface {
    WeightsFor(ctx WeightContext) (Weights, error)
}

type WeightContext struct {
    UserID     string
    Repository string
    Harness    string
}

// Composição de profiles
weights := Merge(
    baseWeights,           // embedded default
    userWeights,           // ~/.harness-downshift/weights.json
    repoWeights,           // .harness-downshift/weights.json no repo
    harnessWeights,        // por harness
)
```

**Por que não agora:** YAGNI. O `weights.json` único com override já entrega personal weights.

### 10.3 Training por Outcome

O training atual usa dataset rotulado. Para aprender com outcome real:

```go
// Futuro
type OutcomeDataset struct {
    Events []OutcomeEvent
}

type OutcomeEvent struct {
    Features     FeatureVector
    SelectedTier Tier
    Success      bool      // task completou sem retry
    Retry        bool      // precisou escalar
    Cost         float64   // custo real
    Duration     time.Duration
}

// Loss function diferente
func OutcomeLoss(pred TierProbabilities, outcome OutcomeEvent) float64 {
    // Penaliza tiers que causaram retry
    // Recompensa tiers que completaram com sucesso e baixo custo
}
```

**Por que não agora:** Dataset diferente, pipeline diferente. Primeiro valida o training rotulado.

### 10.4 Distributed Training

Para escala com múltiplos usuários contribuindo dados:

```go
// Futuro
type FederatedTrainer struct {
    Aggregator WeightAggregator
    Privacy    DifferentialPrivacy
}

// Cada usuário treina local, envia gradientes (não dados)
// Servidor agrega gradientes com privacidade diferencial
```

**Por que não agora:** O projeto é local-first por design. Federado é uma mudança de modelo de negócio.

### 10.5 A/B Testing de Routers

Para comparar routers em produção:

```go
// Futuro
type ABRouter struct {
    A       Router
    B       Router
    Split   float64  // % para B
    Metrics MetricsCollector
}

func (r *ABRouter) Route(ctx context.Context, input RoutingInput) (RoutingDecision, error) {
    if rand.Float64() < r.Split {
        return r.B.Route(ctx, input)
    }
    return r.A.Route(ctx, input)
}
```

**Por que não agora:** Shadow mode (log sem usar) é suficiente para validação inicial.

---

## 11. Critérios de Sucesso

### 11.1 Métricas Obrigatórias

| Métrica | Baseline (legacy) | Target v2 | Critério |
|---------|-------------------|-----------|----------|
| Tier accuracy | ~80% | ≥80% | Não pode regredir |
| `FRONTIER→SMALL` | X% | < X% | **Tem que cair** |
| `FRONTIER→MID` | Y% | ≤ Y% | Não pode subir |
| Risk-weighted loss | — | Menor que baseline | Prova que a loss funciona |

### 11.2 Critério de Go/No-Go

**Se `FRONTIER→SMALL` não cair versus legacy, o v2 não vira default.**

O código fica isolado, o legacy continua, e a única coisa perdida foi tempo de máquina.

---

## 12. Referências

- `internal/core/` — tipos e fluxo do router legacy
- `internal/catalog/` — schema do catalog e Resolver interface
- `internal/benchmark/` — métricas atuais
- `AGENTS.md` — invariantes do projeto
- `LICENSE` — BSL-1.1, copyright headers obrigatórios

---

*Documento criado em 2024-01. Atualizar conforme implementação avança.*
