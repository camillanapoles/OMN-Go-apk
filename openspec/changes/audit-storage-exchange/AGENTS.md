# AGENTS.md — audit-storage-exchange (contrato local da change)

Issue: #33

## Contexto isolado

Uploads e arquivos de usuário: duas vias em `upload_handlers.go` (imagem →
`html/images/`, arquivos de usuário → trees de `config.UserFileTrees`), serving
em `serving.go`, MIME em `config.ResolveContentType`. O stdlib sanitiza o
filename do multipart e `userFileName` sanitiza o caminho de query; a contenção
final é `storage.RelInside`. Ler antes: CLAUDE.md §1 regras 7-8, §5 (nota vs
arquivo), ADR 0006/0007/0008, e a spec desta change.

## Skills ativas

- Globais (manifest `local/skills/manifest.yml`; instalar com
  `bash local/agent-setup/install.sh`): `gitnexus-exploring`,
  `gitnexus-debugging`, `gitnexus-impact-analysis`, `gitnexus-refactoring`,
  `gitnexus-guide`, `gitnexus-cli`, `karpathy-guidelines`.
- Pool (instalar com `bash local/agent-setup/install.sh <id>`):
  - `code-review` (mattpocock/skills) — revisão em dois eixos do contrato Go↔Java de uploads. Uso: antes do commit do contrato.
  - `codebase-design` (mattpocock/skills) — design de módulo para unificar as vias de upload. Uso: na refatoração da duplicação.

## Arquivos permitidos para edição

- `backend/internal/app/upload_handlers.go`, `serving.go`
- `backend/internal/app/upload_test.go`, `serving_test.go`
- `backend/internal/repocheck/` (1 meta-teste novo)
- `android/app/src/main/java/net/basov/omngo/ShareIn.java` (SÓ para manter a
  lista em sinc — de preferência nem tocar)
- Docs: `doc/API.md` (endpoint de upload, se mudar header)

## Proibido editar

- `backend/internal/storage/**` (o domínio não exige mudança lá)
- `backend/internal/config/content_types.go` (MIME não muda no 002)
- `backend/frontend/**`, `android/**` (fora de ShareIn.java)
- `backend/internal/db/**` (backups fora de escopo)

## Comandos obrigatórios antes do commit

1. `npx -y gitnexus@1.6.12 check --cycles`
2. `npx -y gitnexus@1.6.12 detect-changes`
3. PR com Quality Gate + APK Build verdes (`gh run watch`) — Java e Go rodam
   no CI.
4. Bump de versão nos 3 arquivos.

## Proibições do GitNexus

- `serving.go` é caminho de TODOS os assets (inclui KaTeX/fonts vendored):
  o gancho de header de 002 precisa ser por extensão, não global no handler de
  arquivo, senão as fontes/vendored mudam de resposta.
- Não criar segundo resolvedor de nome (`userFileName` único) nem segundo
  teste de contenção (`RelInside` único).
- Não adicionar extensão à lista sem o meta-teste de 001 verde.
