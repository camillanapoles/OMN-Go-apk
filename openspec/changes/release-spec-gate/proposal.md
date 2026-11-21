# Proposal — release-spec-gate

> **Issue:** #42
> **Domínio:** `release` — o pipeline de release do fork (`release.yml`,
> `android-gomobile-release.yml`) e o changelog de versão.
> **Branch da sessão:** `arena/301e1f17-omn-go-apk` · **Commit base:** `ff14c82`
> **Nota:** commit toca workflows e Go (meta-teste) — com bump de versão.

## Cenário atual

O workflow Release dispara a cada push em `main` e cria a tag
`v<APP_VERSION>-f.N` sempre que a versão subiu — a regra do bump por commit
(CLAUDE.md §6) transforma cada PR de código em uma release pequena, com notas
geradas automaticamente para revisores de código. A versão 26.11.35 saiu em
duas releases (`-f.1` e `-f.2`) sem nenhuma alteração do aplicativo entre
elas. As notas de release não falam a língua de quem usa o app. A decisão
está registrada em `doc/decisions/0031-release-after-a-completed-spec.md`.

## Listas identificadas (tags topológicas)

### CONFLITO
- `[RELEASE::CONFLITO::GOVERNANCA::001]` — O gatilho `push: main` do Release
  publica uma release por PR (evidência: tags `v26.11.35-f.1`/`-f.2` da mesma
  versão), e os gatilhos herdados do upstream no build de release
  (`push: [master, DS]`, `pull_request: [master]`) pagam builds redundantes.

### GAP
- `[RELEASE::GAP::USABILIDADE::002]` — As notas de release usam somente
  `generate_release_notes` (créditos de PR) — não existe changelog da versão
  em linguagem da pessoa usuária.
- `[RELEASE::GAP::GOVERNANCA::003]` — Nenhuma verificação determinística de
  "spec concluída" existe no momento da release; a conclusão é implícita, não
  um fato provado no gate (issue fechada, tasks sem pendência com critérios).

## Matriz de Impacto Prévio de Refatoração

| Arquivo/Símbolo | Blast radius | Risco | Testes que guardam |
| --- | --- | --- | --- |
| `release.yml` (gatilho) | toda publicação de release | MÉDIO | `TestReleaseWorkflowRequiresSpecGate` (novo) |
| `android-gomobile-release.yml` (gatilhos herdados) | builds de branch redundantes | BAIXO | idem (novo) |
| notação da release (softprops) | corpo da GitHub Release | BAIXO | `TestReleaseChangelogIsHumanized` (novo) + gate |
| rotina do mantenedor (RUNBOOK/gitops) | operação de release | BAIXO | revisão no PR |
