# 0005. Keep each secret out of the Config page

* Status: accepted
* Version: 26.09.7
* Code: `applyGitServerForm` in `backend/config_fields.go`,
  `gitServerView` and `configPageView` in `backend/templates.go`,
  `backend/frontend/html/js/OMN-Go/omn-go-config.js`

## Context

The server compiles the Config page into a file in the storage
directory. Each reader of the device can open that file. Before 26.09.7,
the page held the admin password, the guest password, each SSH key and
each key password as the value of a box.

The save of the page had a second fault. For each git server slot, the
server wrote all four fields when one of them was not empty. When the
key box was empty, a save that changed only the name wrote an empty key
over the real one.

## Decision

* The page holds no secret. `configPageView` and `gitServerView` have no
  field for a password or an SSH key, and each secret box is empty.
* The button "Show passwords" reads the values from `GET /api/config`.
* Each secret box has the attribute `data-secret`. When the reader types
  in a box, `omn-go-config.js` sets `data-dirty`. Before the save, it
  removes each secret box that is not dirty from the form data.
* An empty secret box thus means "keep the stored value".
* The server writes each field of a git server slot only when the request
  carries that field. This is the same rule as for each other setting.

## Consequences

* To clear a password, the reader must type in the box. An empty box that
  is not dirty changes nothing.
* `TestConfigPageCarriesNoSecret` and `TestConfigPostKeepsAnUnsentGitSecret`
  in `backend/config_secrets_test.go` hold the two halves of the rule.
* `TestSecretAttributeHasAFrontendReader` checks that the page and the
  script use the same attribute name.
