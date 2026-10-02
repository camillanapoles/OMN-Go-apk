// The actions of the controls, run in the REAL JavaScript.
//
// A control of index.html or modals.html has no inline onclick. It names
// its work in data-action, and ONE click listener of omn-go-core.js calls
// the function of that name. See OMN.action in that file.
//
// A FAULT HERE IS A DEAD BUTTON. Nothing throws, and no test of the server
// sees it. These tests send a click to the real listener and read what it
// called.

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const { newPage, run, makeElement } = require('./page-stub.js');

// loadPage answers a page with the two scripts of each note page, and the
// click listeners that they gave to the document.
function loadPage(protocol) {
    const page = newPage();
    if (protocol) page.location.protocol = protocol;
    // A console of its own. omn-go-core.js puts a hook on each console
    // method, and the console of Node is one object for all the tests.
    page.console = { log() {}, info() {}, warn() {}, error() {}, debug() {} };
    const clicks = [];
    page.document.addEventListener = function (type, fn) {
        if (type === 'click') clicks.push(fn);
    };
    for (const file of ['omn-go-core.js', 'omn-go-api.js']) {
        const err = run(page, file);
        assert.strictEqual(err, null, file + ' did not load: ' + (err && err.stack));
    }
    return { page, clicks };
}

// control answers an element with a data-action, and with a data-arg when
// arg is not undefined.
function control(name, arg, tagName) {
    const el = makeElement();
    el.tagName = tagName || 'BUTTON';
    el.getAttribute = function (attr) {
        if (attr === 'data-action') return name;
        if (attr === 'data-arg') return arg === undefined ? null : arg;
        return null;
    };
    return el;
}

// click sends one click on a child of el to each click listener. It
// answers the event.
function click(clicks, el) {
    const event = {
        button: 0, defaultPrevented: false,
        target: { closest: function (sel) { return sel === '[data-action]' ? el : null; } },
        preventDefault: function () { this.defaultPrevented = true; },
    };
    for (const fn of clicks) fn(event);
    return event;
}

test('a click calls the function of the action with the control', () => {
    const { page, clicks } = loadPage();
    const seen = [];
    page.OMN.action('probe', function (el, event) { seen.push([el, event]); });
    const el = control('probe');
    const event = click(clicks, el);
    assert.strictEqual(seen.length, 1, 'the action ran ' + seen.length + ' times');
    assert.strictEqual(seen[0][0], el);
    assert.strictEqual(seen[0][1], event);
});

test('a click outside each control calls nothing', () => {
    const { page, clicks } = loadPage();
    let calls = 0;
    page.OMN.action('probe', function () { calls++; });
    click(clicks, null);
    assert.strictEqual(calls, 0);
});

test('an action with no function writes a warning and does not throw', () => {
    const { page, clicks } = loadPage();
    const warnings = [];
    page.console.warn = function (m) { warnings.push(m); };
    click(clicks, control('no-such-action'));
    assert.strictEqual(warnings.length, 1);
    assert.ok(warnings[0].indexOf('no-such-action') >= 0, warnings[0]);
});

test('a link with an action does not follow its href', () => {
    const { page, clicks } = loadPage();
    page.OMN.action('probe', function () {});
    assert.strictEqual(click(clicks, control('probe', undefined, 'A')).defaultPrevented, true);
    assert.strictEqual(click(clicks, control('probe')).defaultPrevented, false);
});

test('the sync control gives its data-arg to syncAction', () => {
    const { page, clicks } = loadPage();
    const seen = [];
    page.syncAction = function (action) { seen.push(action); };
    click(clicks, control('sync', 'download'));
    click(clicks, control('sync', 'upload'));
    assert.deepStrictEqual(seen, ['download', 'upload']);
});

test('the copy control gives itself to copyQuickNote', () => {
    const { page, clicks } = loadPage();
    let got = null;
    page.copyQuickNote = function (btn) { got = btn; };
    const el = control('quick-note-copy');
    click(clicks, el);
    assert.strictEqual(got, el);
});

test('hide-panel and toggle-panel work on the panel of data-arg', () => {
    const { page, clicks } = loadPage();
    const calls = [];
    const panel = makeElement();
    panel.classList = {
        add: function (c) { calls.push('add ' + c); },
        toggle: function (c) { calls.push('toggle ' + c); },
    };
    page.document.getElementById = function (id) { return id === 'quickPanel' ? panel : null; };
    click(clicks, control('hide-panel', 'quickPanel'));
    click(clicks, control('toggle-panel', 'quickPanel'));
    // An id that the page does not have must not throw. An exported page
    // has no modal.
    click(clicks, control('hide-panel', 'absent'));
    assert.deepStrictEqual(calls, ['add hidden', 'toggle hidden']);
});

test('on a page from disk a server action does not throw', () => {
    const { page, clicks } = loadPage('file:');
    const debug = [];
    page.printDebug = function (name) { debug.push(name); };
    // No lazy file loads on such a page, thus window.syncAction is absent.
    click(clicks, control('sync', 'download'));
    click(clicks, control('new-page'));
    assert.deepStrictEqual(debug, ['syncAction', 'createNewPage']);
});

test('replace-location opens the address of data-arg', () => {
    const { page, clicks } = loadPage();
    const seen = [];
    page.location.replace = function (url) { seen.push(url); };
    click(clicks, control('replace-location', '/Config.html'));
    assert.deepStrictEqual(seen, ['/Config.html']);
});
