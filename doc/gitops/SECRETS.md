# Secrets e variáveis deste fork

Resposta direta à dúvida sobre o `fork-ci.yml`: **os secrets não são
obrigatórios para o CI funcionar**. Sem eles, o workflow gera um keystore
descartável e o APK sai assinado com uma chave de teste, com aviso no
log (`::warning::`). Para release de verdade, configure os secrets
abaixo.

## O que cada workflow usa

| Nome | Tipo | Quem usa | Obrigatório? |
| ---- | ---- | -------- | ------------ |
| `KEYSTORE_BASE64` | secret | `fork-ci.yml`, `android-gomobile-release.yml` | Não para CI; **sim** para release assinado de verdade |
| `KEYSTORE_PASSWORD` | secret | idem | idem |
| `KEY_ALIAS` | secret | idem | idem |
| `KEY_PASSWORD` | secret | idem | idem |
| `FORK_SYNC_TOKEN` | secret | `fork-sync.yml` | Não (sem ele, o sync automático avisa e para; o sync manual continua) |
| `GITLAB_PAT` | secret | `sync-gitlab.yml` | Só se você ativar o mirror para o GitLab |
| `GITLAB_MIRROR_URL` | **variável** | `sync-gitlab.yml` | Não — sem a variável, o mirror fica **desligado** (é o padrão seguro) |

## Como criar o keystore de release (uma vez)

```sh
keytool -genkeypair -v \
  -keystore omn-go.keystore \
  -alias omn-go \
  -keyalg RSA -keysize 4096 -validity 10000 \
  -dname "CN=OMN-Go, O=<seu nome>"
```

Guarde o `omn-go.keystore` fora do git (nunca commite!). Depois:

```sh
base64 -w0 omn-go.keystore   # vira o valor de KEYSTORE_BASE64
```

No GitHub: **Settings → Secrets and variables → Actions → New
repository secret**, crie `KEYSTORE_BASE64`, `KEYSTORE_PASSWORD`,
`KEY_ALIAS` (valor: `omn-go`) e `KEY_PASSWORD`.

> Atenção: perder o keystore significa não conseguir mais atualizar o APK
> instalado nos aparelhos (a assinatura muda). Guarde-o em local seguro.

## FORK_SYNC_TOKEN (sync automático do upstream)

Um PAT (fine-grained) seu com permissão `Contents: write` e `Pull
requests: write` neste repositório. É preciso porque pushes feitos com o
`GITHUB_TOKEN` padrão não disparam outros workflows (os checks do PR de
staging nunca chegariam). Crie em **Settings → Developer settings →
Personal access tokens → Fine-grained tokens**, aponte para
`camillanapoles/OMN-Go-apk` e salve como secret `FORK_SYNC_TOKEN`.

## Mirror para o GitLab (opcional, desligado por padrão)

O `sync-gitlab.yml` original empurrava **tudo** para
`gitlab.com/mvbasov/OMN-Go` (o GitLab do autor do original). Agora ele
só espelha quando você define a variável de repositório
`GITLAB_MIRROR_URL` (ex.: `gitlab.com/camillanapoles/OMN-Go-apk`) e o
secret `GITLAB_PAT` (token do GitLab com write nesse projeto). Espelha
somente `main`, `merge/sync-cnmfs` e as tags `v*`.

```sh
gh variable set GITLAB_MIRROR_URL --body "gitlab.com/<você>/<repo>"
gh secret set GITLAB_PAT   # cola o token do GitLab
```

## Conferindo o que existe hoje

```sh
gh secret list
gh variable list   # ou: gh api repos/camillanapoles/OMN-Go-apk/actions/variables
```
