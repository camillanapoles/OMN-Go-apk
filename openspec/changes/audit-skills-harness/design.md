# Design — audit-skills-harness

## Stack envolvida

- **Sem mudança no produto.** O instalador e o manifest são ferramentas do
  mantenedor. `local/` continua fora do Docker context e do APK.
- `local/skills/manifest.yml` é um link para
  `backend/internal/repocheck/testdata/agent-skills-manifest.yml`. Assim, o
  Gate valida o mesmo conteúdo que o instalador lê, sem incluir `local/` no
  contexto Docker.
- Bash 4+, Git e ferramentas POSIX. O instalador não usa `yq`, npm, Python ou
  dependências de runtime. O meta-teste Go valida um subconjunto YAML fechado.
- `SKILL.md` com frontmatter `name`/`description` e `AGENTS.md` são formatos
  abertos. O instalador atual materializa as skills em `.claude/skills/`;
  adaptadores para outros agentes ficam fora desta entrega.

## Construção (build/test/lint)

- O Quality Gate continua no Fork CI. `repocheck/agent_skills_test.go` valida
  os pins, caminhos, estados e decisões conhecidas sem acessar a rede.
- O instalador baixa cada repositório uma vez por origem e pin, valida o SHA
  do commit, o caminho e o frontmatter. Uma skill externa ausente falha com
  origem, pin e caminho.
- `install.sh` instala o conjunto global por padrão. `--spec <id>` lê a linha
  `Seleção por spec:` do `AGENTS.md` da change e exige que ela coincida com as
  entradas inativas cujo `applies_to` inclui o ID. Qualquer divergência falha
  antes de baixar ou alterar arquivos; só então instala a seleção declarada.
  Sem `--spec`, as skills de estágio inalteradas retornam ao estado global.
- Um marcador local registra origem, pin e checksum. O instalador recusa
  sobrescrever arquivos não gerenciados ou alterados por uma pessoa. As skills
  persistentes ficam em `.claude/skills/`; downloads temporários são removidos
  ao sair. O instalador não edita `CLAUDE.md` nem `AGENTS.md`.
- O manifest aponta para arquivos externos por commit; o repositório não
  armazena cópias de skills de terceiros.

## Governança

- Nenhuma skill global sem origem, pin completo, estado e motivo.
- `karpathy-guidelines` e as seis skills GitNexus são globais. O pacote
  GitNexus declara licença PolyForm-Noncommercial-1.0.0; revisar antes de uso
  comercial.
- `chrisbanes/skills` permanece inativo por conflito com
  `CLAUDE.md` §1 regra 1. `jev-agent` permanece inativo por usar serviço
  externo e exigir `JEV_API_KEY`.
- As specs declaram a seleção de estágio em seus `AGENTS.md`. A seleção não
  ativa o pool inteiro. `security-review` só é selecionada para as specs de
  backend-core, config e storage-exchange.
- A instalação usa formatos abertos e pins completos. Não instala plugins nem
  hooks de terceiros.
