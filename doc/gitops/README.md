# GitOps: o modelo de branches e a governança deste fork

Este fork é `camillanapoles/OMN-Go-apk`, fork de `mvbasov/OMN-Go`. O
fluxo GitOps abaixo define o que cada branch é, quem pode mudar o quê, e
qual build cada branch produz. As regras são aplicadas por **rulesets do
GitHub** (arquivos em `rulesets/`, importados por `import-rulesets.sh`)
e pelo workflow **GitOps Guard** (`.github/workflows/gitops-guard.yml`).

## O modelo de branches

```
omn-go/sync        espelho do original (idêntico a mvbasov/OMN-Go master)
                     ▲ só o workflow Fork Sync (push direto, fast-forward)
                     │
cnmfs/[specs]      branches de especificação/desenvolvimento (continuidade)
arena/[sessoes]    branches de sessão de trabalho (continuidade)
                     │ PRs
                     ▼
merge/sync-cnmfs   STAGING: sempre o merge do original (update) + nosso dev
                     │ PR (nosso build staging)
                     ▼
main               PRODUÇÃO: só recebe merge/sync-cnmfs (nosso build produção)
  │ tag v<versão>-f.N automática → GitHub Release com o APK para instalar
  └── depois: os arena/* que já chegaram em main são deletados (sem acumular)
```

Regras de fluxo (validadas pelo check obrigatório **GitOps Guard**):

| Base                 | Origem permitida                        | Observação |
| -------------------- | --------------------------------------- | ---------- |
| `main`               | apenas `merge/sync-cnmfs`               | produção   |
| `merge/sync-cnmfs`   | `omn-go/sync`, `cnmfs/*`, `arena/*`     | staging = original + dev |
| `omn-go/sync`        | nenhum PR                               | só o Fork Sync atualiza |
| `cnmfs/*`            | `cnmfs/*`, `arena/*`                    | continuidade |
| `arena/*`, outras    | livres                                  | branches de trabalho |

PRs vindos de um fork externo falham no GitOps Guard.

## Os rulesets (governança)

| Ruleset                    | Alvo                        | O que garante |
| -------------------------- | --------------------------- | ------------- |
| `producao-main`            | `main`                      | sem delete, sem force-push, só PR, 1 aprovação, conversas resolvidas, checks obrigatórios **estritos** (branch precisa estar atualizada) |
| `staging-merge-sync-cnmfs` | `merge/sync-cnmfs`          | sem delete, sem force-push, só PR, checks obrigatórios |
| `continuidade-cnmfs-arena` | `cnmfs/**`, `arena/**`      | branch nunca é deletada (continuidade) |
| `historico-cnmfs`          | `cnmfs/**`                  | sem force-push (histórico de specs preservado) |
| `espelho-omn-go-sync`      | `omn-go/sync`               | sem delete, sem force-push (o espelho só anda para frente, idêntico ao original) |
| `tags-releases`            | tags `v*`                   | release é imutável (sem delete/rewrite de tag) |

Checks obrigatórios em `main` e `merge/sync-cnmfs`:

1. **Quality Gate** — `go vet` + `go test` dentro do stage `test` do `Dockerfile` (roda também os testes de estilo/comentários do `repocheck`).
2. **APK Build** — build completo (binários desktop + APK assinado) via stage `export` do `Dockerfile`.
3. **GitOps Guard** — o fluxo de branches da tabela acima.

Break-glass: o papel **Admin** do repositório (você) faz bypass — em
`main`/`merge/sync-cnmfs` apenas via pull request (o bypass fica
registrado na audit log do GitHub); nas branches de continuidade,
espelho e tags, o bypass é `always` para reparos. Em um repo com um
autor só, a aprovação de PR vem do bypass do Admin (o autor de um PR não
pode aprovar o próprio PR).

## A governança dos builds

- **Produção (`main`)**: recebe apenas `merge/sync-cnmfs`, com os três
  checks verdes e a aprovação. Um push de tag `v*` a partir de `main`
  dispara o release build (`android-gomobile-release.yml`). Nunca
  force-push; nunca delete.
- **Staging (`merge/sync-cnmfs`)**: recebe merges do original
  (`omn-go/sync`, pelo Fork Sync automático) e do nosso dev (`cnmfs/*`,
  `arena/*`). Os mesmos checks obrigatórios; o build do staging é o
  primeiro lugar onde o APK sai com as informações do fork
  (`build_commit` + `build_commit_url` apontando para
  `camillanapoles/OMN-Go-apk`).
- **Continuidade (`cnmfs/*`, `arena/*`)**: nunca deletadas; `cnmfs/*`
  não aceita reescrita de histórico. Sessões do Arena vivem em
  `arena/*` e sobem para o dev por PR.

## Como os rulesets são versionados (GitOps)

Os rulesets vivem em `doc/gitops/rulesets/*.json` — um arquivo por
ruleset, com o payload exato da API do GitHub. Para aplicar ou atualizar
(depois de qualquer mudança nos JSONs):

```sh
sh doc/gitops/import-rulesets.sh
```

O script cria o ruleset que falta e atualiza o que já existe
(`gh api` com `POST`/`PUT` em `repos/<repo>/rulesets`). Assim, a
governança tem revisão por PR, histórico no git e pode ser reimportada
em outro fork.

## A automação do fluxo

A cadeia inteira anda sozinha. O token `FORK_SYNC_TOKEN` move todos os
workflows abaixo (ver SECRETS.md):

```
arena/* (CI verde) ➞ cnmfs/dev ➞ merge/sync-cnmfs ➞ main ➞ 🏷️ Release com APK ➞ 🧹 delete do arena/*
```

Cada seta é um PR que só mescla com os checks verdes.

| Workflow | Quando | O que faz |
| -------- | ------ | --------- |
| `fork-ci.yml` | PR (e push) | Quality Gate + APK Build; **auto-merge do PR `arena/*` → `cnmfs/dev`** quando os dois jobs passam (a label `hold` segura um PR) |
| `promote.yml` | **push em `cnmfs/dev`** e **push em `merge/sync-cnmfs`** (automáticos) e dispatch com `target` | abre o PR de promoção e mescla quando os checks passam: `cnmfs/dev` → `merge/sync-cnmfs` (staging) e `merge/sync-cnmfs` → `main` (produção) |
| `release.yml` | **push em `main`** | cria e pusha a tag `v<APP_VERSION>-f.N` (versão de `backend/version.go`) |
| `android-gomobile-release.yml` | push de tag `v*` | builda o APK assinado pelo `Dockerfile` e publica a **GitHub Release** com o `.apk` para instalação |
| `cleanup-branches.yml` | **push em `main`** | deleta os `arena/*` cujo trabalho já chegou em `main` (sem PR aberto e pelo menos um PR mesclado) — as sessões nunca acumulam |
| `governance.yml` | push em `doc/gitops/rulesets/**` (ou dispatch) | reaplica os rulesets via `import-rulesets.sh` — a governança vira GitOps de verdade |
| `fork-sync.yml` | segunda-feira 04:17 UTC ou dispatch | atualiza `omn-go/sync` (idêntico ao upstream) e abre o PR para staging |

O que continua com o ser humano, por desenho:

- **Produção**: a última seta também é automática. O ruleset
  `producao-main` continua pedindo 1 aprovação e segura o merge; o
  workflow espera os builds do PR e mescla com o flag `--admin` do gh —
  o bypass de admin do ruleset em modo *pull request*, registrado na
  audit log. Para trazer o clique humano de volta: remova o bypass do
  ruleset — aí o PR de produção espera aprovação.
- **Segurança da release**: o APK da Release só sai com a assinatura
  real se os secrets `KEYSTORE_*` estiverem configurados (senão sai com
  keystore descartável e aviso no log — ver SECRETS.md).
- **Divergência do espelho**: se `omn-go/sync` divergir do upstream, o
  `fork-sync.yml` falha alto e um humano repara.

## E o resto das regras de build

O audit completo (o que estava quebrado/deficitário e o que foi
corrigido) está na descrição do PR e na mensagem de commit. Resumo:
`fork-ci.yml` foi adaptado ao `Dockerfile` unificado (o `Dockerfile.ci`
do fork sumiu no merge com o original), ganhou os `--build-arg
OMN_GO_COMMIT/OMN_GO_COMMIT_TIME`, os triggers das branches do modelo e
o fallback de keystore sem secrets; `fork-sync.yml` passou a cuidar do
`omn-go/sync`; `sync-gitlab.yml` deixou de empurrar código para o GitLab
do autor do original e agora é opt-in (ver SECRETS.md).
