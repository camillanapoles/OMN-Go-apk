# PROMPT MESTRADO (v2, refinado) — Auditoria Arquitetural e Geração de Specs

> **Instrução de Orquestração Autônoma** para execução no repositório
> `camillanapoles/OMN-Go-apk` (fork de `mvbasov/OMN-Go`).
> Refinado em 2026-11-19 a partir do commit base `4ee14c2` (branch de sessão
> `arena/d9c9b8dd-omn-go-apk`). Todas as chamadas de ferramenta deste documento
> foram **executadas e validadas** no ambiente de destino antes da publicação.
> Diferenças em relação à v1 estão marcadas com **[CORRIGIDO v2]**.

---

# MISSÃO: AUDITORIA ARQUITETURAL E GERAÇÃO DE SPECS (OPENSPEC + GITNEXUS)

Você é o **Agente Arquiteto Orquestrador**. Sua missão é executar um estudo
completo do cenário atual deste repositório usando **GitNexus (CLI local, via
npx)** e a metodologia **Skill-Scout** com 3 frentes de análise paralelas, e
converter todo o diagnóstico em **Especificações Vivas no padrão OpenSpec**,
**GitHub Issues** e **Contratos `AGENTS.md`** por spec.

Regra de ouro: esta auditoria é **não invasiva**. Ela produz apenas
documentação (`openspec/`, `docs/`) e issues. **Nenhum arquivo de código
produção (`backend/`, `android/`, `main_desktop.go`, `Dockerfile`, workflows)
pode ser alterado** nesta execução. Mudanças de código só acontecem depois,
uma issue por worktree/branch, seguindo `doc/gitops/README.md`.

---

## CONTEXTO VALIDADO DO REPOSITÓRIO (leia antes de tudo)

### Stack principal (validada — não é Node/TS/Python)

| Camada | Tecnologia | Onde | Observação dura |
| --- | --- | --- | --- |
| Backend | **Go 1.25/1.26**, módulo `net.basov.omngo`, `CGO_ENABLED=0` | `backend/`, `main_desktop.go` | ~181 arquivos `.go`, ~43k LOC. 3 deps diretas: `goldmark`, `go-git/v5`, `modernc.org/sqlite`. |
| Android | **Java puro** (não Kotlin), **sem AndroidX/AppCompat**, AGP 8.2.2, minSdk 23 | `android/` | AAR via `gomobile bind` (`android/build_aar.sh`, pin `XMOBILE_VERSION`). |
| Frontend | **JS vanilla** (ES5 em `omn-go-compat.js`; IIFE + `window.*`), CSS manual com paleta `--omn-*` | `backend/frontend/` | **Sem build step: sem `package.json`, sem bundler, sem `node_modules`.** Embutido via `go:embed`. |
| Testes | `go test` + `node --test` (JS) + `javac`/`java` (Java) | `backend/internal/`, `backend/frontend/test/`, `android/test/` | Gate único: stage `test` do `Dockerfile` (`go vet ./backend/... && go test ./backend/...`). |
| Build/CI | Docker multistage (imagem `golang:1.26.0-trixie`), GitHub Actions + GitLab CI | `Dockerfile`, `.github/workflows/`, `.gitlab-ci.yml` | Fork CI = quality gate + APK. Releases só por tag (mantenedor). |
| Infra de gov. | Rulesets GitOps + meta-testes que leem o repositório | `doc/gitops/`, `backend/internal/repocheck/` | `repocheck` é **governança executável**: as regras do CLAUDE.md viram testes que falham o build. |

### Modelo de branches (valida no check **GitOps Guard**)

```
omn-go/sync        espelho do upstream (só o workflow Fork Sync mexe)
cnmfs/[specs]      branches de especificação (continuidade, nunca deletadas)
arena/[sessoes]    branches de sessão de trabalho (continuidade)
merge/sync-cnmfs   STAGING (só recebe de omn-go/sync, cnmfs/*, arena/*)
main               PRODUÇÃO (só recebe merge/sync-cnmfs)
```

PRs de fora do fork falham no GitOps Guard. Checks obrigatórios em
`merge/sync-cnmfs` e `main`: **Quality Gate**, **APK Build**, **GitOps Guard**.

### [CORRIGIDO v2] Ambiente de execução (sandbox Arena / host sem toolchain)

| Ferramenta | Disponível aqui? | Consequência para este prompt |
| --- | --- | --- |
| Node 22 / npm / npx | ✅ | gitnexus e openspec rodam via `npx -y pkg@versão`. |
| `gh` (autenticado como `camillanapoles`) | ✅ | Issues, labels e watch de CI direto. |
| GitNexus CLI | ✅ v1.6.12 | `analyze` ~27 s neste repo: 6.173 nós / 20.556 arestas / 523 flows. |
| OpenSpec CLI | ✅ via `@fission-ai/openspec@2.1.0` | **[CORRIGIDO v2]** o pacote `openspec` "puro" do npm é placeholder `0.0.0` — usar sempre `@fission-ai/openspec`. |
| **Go toolchain** | ❌ | `go test`/`go vet` **não rodam no host**. O gate é o CI (`Fork CI`), igual ao que `docs/CONTRIBUTING.md` já prescreve. |
| **Java/JDK, Gradle, Android SDK** | ❌ | Testes Java: só CI. |
| **Docker** | ❌ | `docker buildx build --target test .` indisponível; usar CI. |
| GitNexus como MCP | ❌ não conectado nesta sessão (apps conectados: google_docs, google_drive, linear, notion) | **[CORRIGIDO v2]** invocar **sempre via CLI npx** (o próprio gitnexus é "local CLI and MCP server"). |
| Extensões FTS/VECTOR do grafo | ❌ offline | **[CORRIGIDO v2]** `gitnexus query` retorna vazio sem FTS. Usar `grep` + `context`/`impact`/`trace`/`cypher`. Avisos WARN de FTS no stderr são **esperados** e não são falha. |
| Histórico git | shallow (profundidade 1) | Análise por histórico (`git log`) é limitada ao commit base. Basear-se no grafo e nos docs. |

---

## FASE 0: PRÉ-VOO OBRIGATÓRIO (setup e saneamento)

Rode na raiz do repositório, na **branch de sessão atual** (`arena/*`).
**[CORRIGIDO v2] NÃO crie git worktree** (o ambiente de sessão é preso a uma
branch e nada fora da raiz do workspace persiste). Nada de branch nova aqui.

1. **Ferramentas** (todas devem responder antes de continuar):
   ```bash
   node --version && npx --version
   gh auth status | head -4
   npx -y gitnexus@1.6.12 --version
   npx -y @fission-ai/openspec@2.1.0 --version
   ```
2. **Indexar o grafo** — **[CORRIGIDO v2] com `--skip-fts` (extensão FTS não
   instalável offline) e `--skip-agents-md` (sem isso o gitnexus REESCREVE
   `CLAUDE.md` e cria `AGENTS.md`/`.claude/` automáticos — poluição proibida):
   ```bash
   npx -y gitnexus@1.6.12 analyze . --skip-fts --skip-agents-md
   npx -y gitnexus@1.6.12 check --cycles
   ```
   Saída esperada: ~6k nós / ~20k arestas; `check --cycles` deve dizer
   "No circular imports found" (se apontar ciclos, isso vira achado
   `CONFLITO` imediato). Linhas `WARN ... FTS extension unavailable` no
   stderr são esperadas — ignore-as.
3. **Infra OpenSpec** (se `openspec/` não existe ainda):
   ```bash
   npx -y @fission-ai/openspec@2.1.0 init --tools none --no-animation
   ```
   **[CORRIGIDO v2]** `init` **não tem** flag `--non-interactive`; a forma
   acima é não-interativa de fato. Cria `openspec/config.yaml`,
   `openspec/specs/.gitkeep`, `openspec/changes/archive/.gitkeep` — comite-os
   como infra da auditoria.
4. **Rótulos do GitHub** — **[CORRIGIDO v2]** o repo só tem os labels padrão;
   criar antes de qualquer issue (o `gh issue create --label` falha sem isto):
   ```bash
   gh label create "spec:draft" --color 1D76DB --description "Spec OpenSpec aguardando refinamento" || true
   gh label create "audit"       --color FBCA04 --description "Item vindo da auditoria arquitetural" || true
   gh label create "dom:backend-core" --color 0E8A16 --description "Domínio: backend/internal/app + server/routes/session" || true
   gh label create "dom:storage"      --color 0E8A16 --description "Domínio: storage, assets, exchange" || true
   gh label create "dom:render-search" --color 0E8A16 --description "Domínio: render, noteheader, textmatch, search" || true
   gh label create "dom:sync-git"     --color 0E8A16 --description "Domínio: gitsync (pull/push/host keys)" || true
   gh label create "dom:db"           --color 0E8A16 --description "Domínio: SQLite, backups, /api/sql" || true
   gh label create "dom:config"       --color 0E8A16 --description "Domínio: config.Store, campos, MIME" || true
   gh label create "dom:frontend-js"  --color 5319E7 --description "Domínio: JS/CSS/templates do frontend" || true
   gh label create "dom:android"      --color 5319E7 --description "Domínio: Java/Gradle/gomobile" || true
   gh label create "dom:build-ci"     --color D93F0B --description "Domínio: Dockerfile, workflows, pipelines" || true
   gh label create "dom:docs-gov"     --color C2E0C6 --description "Domínio: docs, decisões, repocheck" || true
   ```
   A lista de domínios é candidata — ajuste na FASE 2, criando/removendo
   labels para que **cada spec tenha exatamente um label `dom:*`**.
5. **Higiene**: confirme que `.gitignore` contém `.gitnexus/` (o índice ocupa
   ~84 MB e nunca vai para o Git). Se faltar, adicione nesta execução.
6. **Decisão registrada sobre bump de versão** (regra §6 do `CLAUDE.md`):
   commits **100% documentação/openspec** desta auditoria **não** fazem bump de
   `APP_VERSION`; registre essa exceção no topo de cada `proposal.md`. Se um
   commit tocar qualquer arquivo de `backend/`, `android/` ou build, aplique o
   bump completo nos 3 arquivos (`backend/version.go`,
   `android/app/build.gradle`, linha "State of the tree" de `doc/CODE_MAP.md`).

---

## FASE S: SKILLS & HARNESS (PRIORIDADE MÁXIMA — execute ANTES das FASEs 1-3)

Spec dedicada: `openspec/changes/audit-skills-harness/`. Execute esta fase
antes da descoberta: ela otimiza os agentes que executarão o resto da
auditoria.

### S.1 — Validar skill e frameworks GLOBAIS (todo o workspace)

- Fontes externas candidatas (validadas em 2026-11-19/20 — revalide com
  `fetch_page`/`git ls-remote` antes de instalar):
  - `multica-ai/andrej-karpathy-skills` — guidelines gerais (4 princípios
    Karpathy); formato `skills/<id>/SKILL.md` + `.claude-plugin` + Cursor.
  - `mattpocock/skills` — eng. software geral; `.agents/`, marketplace, CI
    valida frontmatter, instala em 9 agentes.
  - `chrisbanes/skills` — Android/Kotlin/Compose; **triagem obrigatória**:
    Kotlin/Compose colidem com CLAUDE.md §1 regra 1 (sem AndroidX, Java puro)
    — só workflow/build/review genéricos podem virar global.
  - `affaan-m/ECC` — harness de agentes (skills, instincts, memory, security,
    research-first); instalar seletivas, nunca o repo.
- Critério de validade: formato aberto SKILL.md (frontmatter `name`/
  `description`), atividade recente ou versão pinada estável, e ZERO conflito
  com `CLAUDE.md` §1. Falhou o critério → entra no pool como `inactive` com
  `reason`, ou é rejeitada.

### S.2 — Instalação REPLICÁVEL (não ad-hoc)

- Toda habilidade é ativo versionado: tree fonte em `local/skills/`
  (próprias) + `local/skills/manifest.yml` (`id/source/pin/status/reason/
  applies_to`) para as externas — **pin de commit obrigatório**, nunca
  material gerado por ferramenta sem registro.
- Instalador único e idempotente: `bash local/agent-setup/install.sh`
  (bash+git+curl apenas, Termux-ok) materializa em `.claude/skills/` e nos
  formatos dos demais agentes. Uma máquina nova fica completa com
  `git clone && bash local/agent-setup/install.sh`. Reexecutar não duplica.
- **[CORRIGIDO v2]** O `.claude/` auto-gerado pelo gitnexus continua fora do
  git; o que versiona é o tree em `local/skills/` — o install.sh é a única
  via de materialização.

### S.3 — Toda habilidade nova é AUDITADA antes de virar global

- Fluxo fixo: candidata entra **inativa** no manifest com origem+pin+motivo →
  auditoria cruzando com `CLAUDE.md` §1/§3-§9 e specs abertas → decisão
  `global` / `per-domain` / `rejected` registrada no manifest → meta-teste
  (`repocheck`) falha o Gate com global sem `reason`/`pin`.
- O que é global ativa em todo workspace (todas as máquinas e worktrees);
  o resto fica no pool inativo.

### S.4 — Seleção por-spec do pool inativo

- Para cada spec, além das globais já ativas, a instrução da spec seleciona do
  pool inativo as skills úteis ao domínio focado e registra a seleção na
  seção "Skills ativas" do `AGENTS.md` da spec (com justificativa ou
  "nenhuma adicional"). A seleção é auditável no PR da issue.

### S.5 — Princípios de INTEROPERABILIDADE (obrigatórios)

- Formatos abertos (SKILL.md + `AGENTS.md`), um único tree fonte por projeto,
  instalação replicável de qualquer máquina, seleção declarada (auditorável
  em PR), fonte terceira sempre pinada e citada. Zero lock-in de vendor:
  a mesma skill deve instalar em Claude Code, Codex, Cursor, OpenCode e
  afins, ou não entra.
- Resultado esperado: cada projeto/workspace/spec com `AGENTS.md` próprio
  (como instruído na FASE 3) **+ otimizado** pelas skills globais e pela
  seleção por-spec das 4 fontes acima.

---

## BASE DE VERDADE (leitura obrigatória antes de emitir qualquer achado)

Nenhum achado pode ser emitido sem antes ler e cruzar com:

1. `CLAUDE.md` — contrato vigente: 12 constraints fixas (§1), mapa do repo
   (§2), regras Go (§3), frontend (§4), notas (§5), versão/release (§6),
   commits (§7), testes (§8), build/CI (§9), STE (§10).
2. `doc/CODE_MAP.md` — **o CodeMap já existe** (feature → arquivo, versão do
   tree na linha "State of the tree"). É o mapa de referência; não recriar.
3. `docs/CODEMAPS/*.md` — codemaps do fork (architecture, backend, data,
   dependencies, frontend; base `d9f6c82`).
4. `doc/decisions/0001…0029` — 29 ADRs. Um comportamento documentado num ADR
   **não é bug**: é decisão.
5. `doc/API.md` (referência de endpoints), `doc/TESTING.md`, `doc/TERMINOLOGY.md`.
6. `backend/internal/repocheck/*_test.go` — as regras transversais **já
   automatizadas** (`TestImportLayers`, `TestGroupsUseOnlyLowerLayers`,
   `TestNoBare64BitAtomics`, `TestNoDirectLogPrintf`, `TestCommentShare`,
   `TestBaseline_RouteSet`, `TestAPIRouteTable`, `TestVersionIsTheSameInEachFile`,
   `pipelines_test.go` etc.). Um "gap" que um desses testes já cobre é
   **falso positivo**.
7. `doc/gitops/README.md`, `docs/RUNBOOK.md`, `docs/CONTRIBUTING.md` — fluxo
   GitOps, segredos, loop de contribuição (R1–R9).

### Lista de desvios intencionais (NÃO emitir como achado)

Documentados em `CLAUDE.md` §1 e ADRs — flagar qualquer um deles é erro:

- `html.WithUnsafe()` ligado (script em nota roda) — trade deliberado (§5).
- Tabela de fold duplicada de propósito (Go `textmatch.FoldTable` ↔ JS
  `OMN_FOLD_TABLE`) com teste que compara as duas (§4).
- Regra `local-` com duas implementações de propósito (`storage.IsLocalOnlyPath`
  e `gitsync.GitignoreLocalOnlyPattern`) (§1, regra 9).
- `log.Printf` em exatamente 2 arquivos (`render/templates.go`,
  `search/sections.go`) — exceções documentadas (§3).
- SEM AndroidX, SEM `html/template`, SEM Tailwind/React/marked, frontend sem
  build step — constraints fixas, não débitos.
- `render.Fill()` com `%%PLACEHOLDER%%` em vez de templates Go.
- Leitor admin sempre liberado em `127.0.0.1`/`::1` (bypass local, §1 regra 10).
- Nota sem extensão conhecida = nota (última extensão decide, §5).
- `.html` compilado sempre com `.md` fonte; `storage.DropCompiledPages` na
  troca de versão (ADR 0026).
- `master` é a branch de trabalho do *upstream*; neste fork quem governa é o
  modelo GitOps do `doc/gitops/README.md`.

---

## FASE 1: DESCOBERTA E MAPEAMENTO PARALELO (SKILL-SCOUT)

Dispare **3 frentes de análise em paralelo**. Cada frente usa o grafo GitNexus
já indexado na FASE 0 + leitura estática. Registre evidência (arquivo:linha)
para tudo.

### Comandos GitNexus validados (use exatamente estes)

**[CORRIGIDO v2]** `gitnexus query` está **banido** nesta configuração (FTS
offline ⇒ resultado sempre vazio). Para busca textual use `grep -rn`; para o
grafo use os comandos abaixo. Símbolos homônimos exigem desambiguação por UID
(formato `Kind:file:symbol`, ex.: há dois `StartServer` — um em
`backend/backend.go`, outro em `backend/internal/app/server.go`):

```bash
# Contexto 360° de um símbolo (chamadores, chamados, fluxos)
npx -y gitnexus@1.6.12 context "Function:backend/internal/app/server.go:StartServer"

# Blast radius de refatoração (sempre com --uid para resultado único)
npx -y gitnexus@1.6.12 impact "Function:backend/internal/app/server.go:StartServer"

# Caminho dirigido mais curto entre dois símbolos
npx -y gitnexus@1.6.12 trace "Function:backend/backend.go:StartServer" "Function:backend/internal/app/server.go:StartServer"

# Consulta crua ao grafo (tabelas: File, Folder, Class, Function, Method,
# Route, Process, Community, Module + rel CodeRelation)
npx -y gitnexus@1.6.12 cypher "MATCH (f:Function)-[r:CodeRelation]->(g:Function) RETURN f.name, g.name LIMIT 25"

# Verificação estrutural (ciclos) e mapeamento de um diff para símbolos
npx -y gitnexus@1.6.12 check --cycles
npx -y gitnexus@1.6.12 detect-changes   # após qualquer mudança no tree
```

### Agente 1 — Engenharia de Software & Code Quality

- Grafo: clusters (`Community`), funções mais conectadas (`cypher` com
  `count`), blast radius dos hotspots via `impact --uid`.
- Buscar: **Erros** (bugs lógicos, `err` ignorado sem `_ =` ou wrap, exceções
  não tratadas em Java/JS), **Conflitos** (acoplamento fora das camadas
  `importLayers`, condições de corrida — ver locks de `config.Store`, `logx`
  e atomicidade §1 regra 4), **Gaps** (pacote/função sem teste, rota fora de
  `TestBaseline_RouteSet`/`doc/API.md`), **Melhorias de Código &
  Ciclomaticidade** (funções gigantes, duplicação, código morto).
- Ferramentas estáticas disponíveis no host: `grep`/`rg` (padrões:
  `log.Printf(`, `atomic.AddInt64(`, `html/template`, `alert(`, `confirm(`,
  `innerHTML`, `onclick=`, `mime.TypeByExtension` — cada hit deve concordar
  com a regra ou é achado). **`go vet` não roda aqui** — anote e deixe para o
  Quality Gate do CI.

### Agente 2 — Design Thinking & Usabilidade (DX/UX)

- Fluxos de entrada/saída: páginas (`systemPages`, `renderPage`), editor
  (`omn-go-editor.js`), Config (`config_page.html` + `omn-go-config.js`),
  dialogs (`omn-go-dialog.js` — `omnAlert/omnConfirm/omnPrompt`).
- Mensagens de erro: texto das respostas (`writeJSONError`, `http.Error`),
  coerência com `omnNotice`/`omnNoticeAfterLoad`, acessibilidade dos dialogs,
  consistência `data-action`/`data-arg`, temas (`data-theme`).
- DX do repositório como produto: clareza de `doc/API.md`, feedback de erro do
  CI (RUNBOOK §problemas conhecidos), onboarding em `docs/CONTRIBUTING.md`.
- Identificar: **Gaps de Usabilidade**, **Conflitos de Fluxo** (ex.: ação
  visível sem rota, texto de UI fora do vocabulário de `doc/TERMINOLOGY.md`),
  **Melhorias de Usabilidade/DX**.

### Agente 3 — Interoperabilidade, Segurança & Governança

- Variáveis de ambiente/segredos: `doc/gitops/SECRETS.md` vs workflows
  (`KEYSTORE_BASE64`, `KEYSTORE_PASSWORD`, `KEY_ALIAS`, `KEY_PASSWORD`,
  `FORK_SYNC_TOKEN`) — nenhum segredo em arquivo do tree.
- Limites de sistema já fixados (verificar que continuam coerentes): corpo
  `/api/sql` ≤ 1 MiB, ≤ 500 statements, nome de DB `^[A-Za-z0-9_-]{1,64}$`,
  storage em `/storage/emulated/0/Android/media/<applicationId>/`, LAN
  binding 127.0.0.1 por padrão, `/api/logs` admin-only, CSRF/origin guard
  (`request_guard.go`, ADR 0017).
- Contratos entre módulos: camadas `importLayers` (repocheck) vs import real
  no grafo; contrato gomobile (7 funções de `backend/backend.go`); portas
  duplicadas de propósito com teste (fold table, log filter).
- Dívidas de infra: acordos de pipeline (`pipelines_test.go` — Go/NDK/
  build-tools/Gradle/x-mobile idênticos entre Dockerfile, recipe F-Droid,
  `build_aar.sh`, Gradle), divergências `.gitlab-ci.yml` vs workflows,
  `.dockerignore` vs diretórios novos (`docs/`, `openspec/` entram no
  `COPY . .` — candidate a exclusão como MELHORIA-GOVERNANCA).
- Identificar: **Vulnerabilidades/Segurança**, **Gaps de Interoperabilidade**,
  **Melhorias de Arquitetura/Construção**.

---

## FASE 2: CORRELAÇÃO, IMPACTO PRÉVIO E SELF-REFLECTION

Antes de escrever qualquer arquivo:

1. **Blast radius por achado**: para cada símbolo citado num achado, rode
   `npx -y gitnexus@1.6.12 impact <uid>` e registre os impacted nodes no
   campo *Where* do item. Achado sem blast radius mapeado **não passa** para a
   FASE 3.
2. **Agrupamento topológico por domínio (Spec)**: agrupe os achados nos
   domínios candidatos da FASE 0 (ajuste a lista se um domínio ficar vazio ou
   atravessar demais — um domínio vago demais vira dois). Cada domínio vira
   UMA spec `audit-<dominio>` e UMA issue.
3. **Taxonomia de Tags Topológicas** — cada achado recebe:
   `[<SPEC_DOMINIO>::<CATEGORIA>::<TIPO_ESPECIFICO>::<ID>]`
   - Categorias: `GAP`, `CONFLITO`, `ERRO`, `MELHORIA`.
   - Tipos: `CODIGO`, `USABILIDADE`, `INTEROPERABILIDADE`, `CICLOMATICIDADE`,
     `SEGURANCA`, `GOVERNANCA`.
   - ID sequencial por domínio+categoria+tipo (001, 002…).
   - Exemplo real deste repo: `[SYNC-GIT::CONFLITO::INTEROPERABILIDADE::001]`.
4. **Loop de validação (refaça e verifique)** — passe obrigatoriamente por:
   - *Cobertura*: confira a árvore de diretórios críticos (`backend/internal/`
     pacote a pacote, `android/app/src/main/java/net/basov/omngo/` classe a
     classe, `backend/frontend/html/js/OMN-Go/` script a script, workflows)
     contra a lista de achados. Nenhum arquivo das listas de §2 do CLAUDE.md e
     §5 de `doc/CODE_MAP.md` pode ficar sem dono em pelo menos um domínio.
   - *Duplicidade*: nenhum símbolo em dois itens da mesma categoria; itens que
     se sobrepõem viram um item com múltiplas evidências.
   - *Anti-falso-positivo*: cruze item a item com a "Lista de desvios
     intencionais" e com os ADRs. Item contraditado por ADR = descartado
     (ou reclassificado como MELHORIA-GOVERNANCA de documentação).
   - *Viabilidade de validação*: cada item precisa de um critério verificável
     por teste (Go/JS/Java no CI) ou por meta-teste do `repocheck`. Sem isso,
     reescreva o item até ter critério — é o padrão M==N do fork.

---

## FASE 3: MATERIALIZAÇÃO NO PADRÃO OPENSPEC + ISSUES + AGENTS.MD

Para **cada domínio** com achados, crie a change no OpenSpec, o contrato local
e a issue. Estrutura exata (compatível com o schema `spec-driven` do
OpenSpec 2.1.0):

```
openspec/
├── config.yaml                      (da FASE 0)
├── specs/.gitkeep
└── changes/
    ├── archive/.gitkeep
    └── audit-<dominio>/             (ex.: audit-sync-git)
        ├── proposal.md
        ├── design.md
        ├── tasks.md
        ├── AGENTS.md
        └── specs/<dominio>/
            └── spec.md
```

### 1. `proposal.md` (Estudo de Cenário e Listas Topológicas)

- Cabeçalho: domínio, branch da sessão, commit base, **número da GitHub Issue
  (preencher depois do passo 3 — deixe a linha `Issue: _pendente_`)**.
- Resumo do cenário atual do domínio (o que existe, como se conecta ao grafo —
  cite contagens do GitNexus: nós, arestas, clusters do domínio).
- **N listas** separadas por categoria (`GAP`, `CONFLITO`, `ERRO`,
  `MELHORIA [subtipo]`), cada item com sua **Tag Topológica** e uma linha de
  evidência (`arquivo:linha — símbolo`).
- Matriz de **Impacto Prévio de Refatoração** (arquivo/símbolo afetado |
  blast radius do GitNexus | risco `ALTO/MEDIO/BAIXO` | testes que guardam a
  mudança — nomeie os testes do `repocheck`/pacote).

### 2. `specs/<dominio>/spec.md` (Detalhamento Técnico em 5W1H)

Para **cada item** de todas as categorias, um bloco obrigatório:

```markdown
### [<SPEC_DOMINIO>::<CATEGORIA>::<TIPO>::<ID>] — <título curto>

- **What (O quê):** o problema/gap/melhoria exato, com a tag.
- **Why (Por quê):** impacto técnico, de negócio, segurança ou usabilidade.
- **Where (Onde):** arquivos:linhas, símbolos e nós do grafo GitNexus
  (UIDs) + blast radius (`impact`).
- **When (Quando):** build, runtime, deploy ou interação (qual momento do
  ciclo manifesta o problema).
- **Who (Quem):** módulo/persona impactada (ex.: `package app`, `gitsync`,
  usuário Android WebView, mantenedor do fork).
- **How (Como):** plano exato de refatoração + mitigação de impacto +
  critério verificável (M==N) referenciando o teste que provará.
```

### 3. `design.md` (Stack, Construção e Governança)

- **Stack**: Go 1.25/1.26 (3 deps diretas, CGO off), Java sem AndroidX, JS
  vanilla sem build, Docker multistage como build de referência.
- **Construção**: `go vet ./backend/... && go test ./backend/...` é o gate
  (roda no CI); testes JS com `node --test` via `repocheck/js_test.go`; Java
  via `repocheck/java_test.go`; meta-testes do `repocheck` são lint — respeite
  `TestCommentShare` (≤ 20% comentários) e o formato de commit do §7
  (`type(scope): Sentence. vYY.MM.S`, ≤ 80 chars, STE).
- **Governança**: critérios de aceite para merge (Quality Gate + APK Build +
  GitOps Guard verdes; PR para `merge/sync-cnmfs` ou `cnmfs/*`; conversas
  resolvidas); isolamento (uma issue = uma branch de trabalho futura
  `cnmfs/<slug>` ou `arena/<slug>`); restrições (nada de segunda
  implementação das autoridades da §1 regra 7; nada de nova dependência Go
  sem justificativa; tags só pelo mantenedor).

### 4. `tasks.md` (Checklist de Verificação e Validação)

Checklist `[ ]` agrupado por prioridade (`P0` segurança/dados, `P1` correto,
`P2` qualidade, `P3` DX). Para cada tarefa:

```markdown
- [ ] <ação>
  - **Verificação:** como testar objetivamente (comando/teste exato; se só
    roda no CI, diga "Quality Gate do Fork CI" e qual subteste).
  - **Validação:** como provar impacto colateral zero (blast radius vazio /
    golden sets de `baseline_test.go` inalterados / `TestBaseline_RouteSet`
    igual / binário dentro do baseline de `binary_size_baseline.json`).
```

### 5. `AGENTS.md` da spec (contrato auto-contido da change)

Dentro de `openspec/changes/audit-<dominio>/`:

- **Contexto isolado**: 5–10 linhas do domínio + link para `CLAUDE.md`,
  `doc/CODE_MAP.md` e ADRs que regem o domínio.
- **Arquivos permitidos para edição** (escopo estrito, caminho a caminho) e
  **arquivos proibidos** (ex.: para `audit-sync-git`: só
  `backend/internal/gitsync/**`, seu `*_app.go`, testes do pacote, docs do
  domínio — **nunca** `backend/backend.go` (facade), `Dockerfile`, workflows).
- **Comandos obrigatórios de validação antes do commit** (neste ambiente, o
  gate local que existe + o CI): `npx -y gitnexus@1.6.12 check --cycles`,
  `npx -y gitnexus@1.6.12 detect-changes`, e `gh run watch` no **Fork CI**
  do PR (`go vet`+`go test` rodam lá, não no host).
- **Proibições explícitas baseadas no GitNexus**: liste os símbolos de alto
  blast radius do domínio (saída do `impact`) que exigem meta-teste atualizado
  em conjunto; e as regras da §1 regra 7 (uma autoridade por decisão).

### 6. Converter cada spec em GitHub Issue referenciada

```bash
gh issue create \
  --title "[SPEC][<DOMINIO>] Auditoria e Refatoração: <resumo>" \
  --label "spec:draft,audit,dom:<dominio>" \
  --body-file openspec/changes/audit-<dominio>/issue-body.md
```

- Gere `issue-body.md` temporário com: link direto para o diretório
  `openspec/changes/audit-<dominio>/` (URL do blob na branch da sessão),
  resumo das tags topológicas contempladas (contagem por categoria) e
  checklist macro (P0→P3 de `tasks.md`).
- **[CORRIGIDO v2]** labels são criados na FASE 0 — não dependa de
  auto-criação.
- Feche a rastreabilidade bidirecional: edite `proposal.md` e o `AGENTS.md`
  da spec, trocando `Issue: _pendente_` por `Issue: #<número>`, e comite
  (`docs(audit): Link audit-<dominio> specs to issue #N.` — sem bump, ver
  FASE 0.6).

---

## FASE 4: CONFORMIDADE ANTES DE ENCERRAR (novo — obrigatório)

1. `git status --porcelain` não pode conter `.gitnexus/`, `.claude/`,
   `AGENTS.md` na raiz, nem nenhum arquivo de código alterado.
2. Diff da execução contém apenas: `openspec/**`, `docs/AUDIT_PROMPT.md` (se
   ajustada), `.gitignore` (entry de `.gitnexus/`), eventuais labels/issues no
   GitHub.
3. Se a execução gerar PR: destino `merge/sync-cnmfs` (ou `cnmfs/*`), título
   no formato do fork, e os 3 checks obrigatórios verdes
   (`gh run list --workflow "Fork CI" --branch <branch> --limit 1`).
4. Commit messages no padrão §7 do CLAUDE.md (STE, ≤ 80 chars, bullets).

---

## REGRA DE SAÍDA FINAL

Ao concluir todas as fases, apresente apenas:

1. **Mapa Topológico Geral** — árvore de specs → listas por tipo → contagem de
   itens, no formato:
   ```
   audit-sync-git (dom:sync-git)
   ├── GAP          :: INTEROPERABILIDADE :: 2   [SYNC-GIT::GAP::INTEROPERABILIDADE::001..002]
   ├── CONFLITO     :: SEGURANCA         :: 1   [...]
   ├── ERRO         :: CODIGO            :: 0
   └── MELHORIA     :: CODIGO            :: 3   [...]
   ```
2. **Tabela de Rastreabilidade**:
   `Tag da Spec | Diretório OpenSpec | AGENTS.md | GitHub Issue #`.

---

## ANEXO A — Execução fora do Arena (host local)

```bash
# 1. Indexar (skip FTS somente se offline; com internet, rode sem --skip-fts)
npx -y gitnexus@1.6.12 analyze . --skip-agents-md
# 2. Este prompt como arquivo:
claude -p "$(cat docs/AUDIT_PROMPT.md)" --allowedTools "Bash,Read,Write,Edit,Grep,Glob"
# 3. Opcional com isolamento por worktree (fora do Arena apenas):
git worktree add ../wt-audit-openspec -b cnmfs/openspec-audit
#    (branches de continuidade do fork começam com cnmfs/ ou arena/ — nunca docs/)
```

## ANEXO B — Limitações conhecidas desta configuração

- `gitnexus query` vazio (FTS/VECTOR offline) — usar `grep` + `context`/
  `impact`/`trace`/`cypher`. Avisos `WARN ... fts` no stderr são ruído
  esperado, não erro.
- `context`/`impact` sem UID podem responder `status: "ambiguous"` quando
  símbolos homônimos existem (ex.: dois `StartServer`, dois `renderPage`).
  Sempre desambigue com o UID completo.
- Repo shallow (1 commit): análise por histórico indisponível; baseie-se no
  grafo + `doc/CODE_MAP.md`.
- Índice GitNexus = ~84 MB em `.gitnexus/` — regenerável em ~30 s a qualquer
  momento; nunca versionar.
