# Tasks — audit-skills-harness (PRIORIDADE MÁXIMA — execute primeiro)

## P0 (replicabilidade — fundação dos agentes)

- [x] Criar o manifest apresentado em `local/skills/manifest.yml`, com as cinco fontes validadas, seis skills GitNexus e a skill geral Karpathy.
  - **Verificação:** todas as entries têm origem, commit completo, estado e motivo; skills de terceiros não são armazenadas no repositório.
  - **Validação:** `local/` continua fora do contexto Docker; instalação não muda `git status`.
- [x] Criar `local/agent-setup/install.sh` idempotente.
  - **Verificação:** duas instalações mantêm os mesmos hashes; `--spec` instala `security-review` somente quando manifest e `AGENTS.md` coincidem; sem `--spec`, remove a skill de estágio inalterada e preserva arquivos/symlinks não gerenciados.
  - **Validação:** Bash e Git apenas para dependências externas; skills persistentes ficam somente em `.claude/skills/`, downloads temporários são removidos ao sair, e o script não altera `backend/`, `android/`, `CLAUDE.md` ou `AGENTS.md`.

## P1 (gate de auditoria — governança executável)

- [x] Adicionar `repocheck/agent_skills_test.go` para validar o manifest e as seleções declaradas sem rede.
  - **Verificação:** entry sem motivo, pin completo, origem ou caminho aprovado falha; os oito `AGENTS.md` coincidem com `applies_to` e a mensagem identifica a divergência.
  - **Validação:** `go vet` e `go test` dos arquivos do meta-teste passaram isoladamente; origem, pin e caminho remoto são conferidos no instalador.
- [ ] Executar `go vet ./backend/...` e `go test ./backend/...` no Quality Gate.
  - **Verificação:** ambos os comandos concluem sem erros.
  - **Validação:** PR do branch atual recebe o check `Quality Gate` verde; localmente, sem `go.sum` ou cache dos módulos, o modo offline falha antes de compilar o backend.
- [x] Triar as skills externas contra `CLAUDE.md` §1 e os requisitos do harness.
  - **Verificação:** Kotlin/Compose permanece `inactive` por conflito com §1 regra 1; `jev-agent` permanece `inactive` por serviço externo e API key; opções de build/review ficam registradas.
  - **Validação:** cada entry tem decisão, motivo e pin; nenhuma skill é ativada sem seleção.

## P2 (seleção por spec)

- [x] Adicionar a seção "Skills ativas" aos oito `AGENTS.md` das specs.
  - **Verificação:** cada arquivo declara as skills globais e justifica a seleção de estágio ou declara que não há skill adicional.
  - **Validação:** as seleções de `security-review` coincidem com `applies_to` no manifest; as skills conflitantes continuam inativas.
- [x] Documentar no RUNBOOK a rotina de leitura, seleção e desativação.
  - **Verificação:** a seção "Agent skills" fornece comandos exatos para instalar globais, selecionar uma spec e retornar ao conjunto global.
  - **Validação:** a rotina mantém `local/` fora do build e pede revisão de licença para uso comercial das skills GitNexus.

**Validação pendente:** os testes isolados do meta-teste passam com Go 1.26.0, mas o gate completo ainda não compilou: faltam `go.sum` e o cache dos módulos de `go.mod`, e `GOPROXY=off` impede resolução externa. A issue não está pronta para merge até `go vet ./backend/...` e `go test ./backend/...` passarem no Quality Gate.
