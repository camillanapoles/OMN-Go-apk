# Tasks — release-spec-gate

## P0 (gatilho e gate)

- [x] Trocar o gatilho do Release para `workflow_dispatch` com a issue e
  rodar o gate antes da tag (CONFLITO::GOVERNANCA::001).
  - **Verificação:** dispatch com o número de uma issue aberta falha no
    passo do gate com a mensagem exata do que falta; após o merge deste PR,
    nenhum push em `main` cria run do Release nem tag.
  - **Validação:** `TestReleaseWorkflowRequiresSpecGate` verde no Fork CI;
    os três checks obrigatórios dos rulesets permanecem os mesmos.
- [x] Remover os gatilhos de branch do build de release
  (`push: [master, DS]` e `pull_request: [master]`)
  (CONFLITO::GOVERNANCA::001).
  - **Verificação:** push em branch de trabalho não enfileira run do
    `Build OMN-Go`; `workflow_dispatch` continua buildando artefatos de teste.
  - **Validação:** Fork CI e GitOps Guard inalterados; nenhum ruleset perde
    check obrigatório.

## P1 (changelog humanizado)

- [x] Publicar `doc/changelog/<versão>.md` como corpo da release
  (GAP::USABILIDADE::002).
  - **Verificação:** a release gerada pela tag traz as notas em português da
    pessoa usuária antes das notas automáticas do GitHub.
  - **Validação:** `TestReleaseChangelogIsHumanized` verde (formato •,
    mínimo de palavras, sem hash, sem arquivo, sem jargão) para a versão
    vigente.

## P1 (continuidade)

- [x] Meta-testes do repocheck prendendo as três regras
  (GAP::GOVERNANCA::003).
  - **Verificação:** num branch de teste, reverter o gatilho do Release para
    push ou mover o gate depois da tag quebra o Fork CI com a mensagem do
    meta-teste; apagar o changelog da versão vigente quebra o Fork CI.
  - **Validação:** `go vet` verde; nenhum outro meta-teste afetado.
