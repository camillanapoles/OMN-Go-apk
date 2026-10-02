// A page with a real document, for the tests that press a control.
//
// page-stub.js builds the window. This file gives that window the document
// of mini-dom.js, the markup of the real templates, and a server that the
// test writes. A test then loads the shipped scripts, presses a control and
// reads the page and the requests.
//
// THE MARKUP IS THE MARKUP OF THE TEMPLATES. shellBody reads index.html and
// modals.html, the same files that the server sends. A control that a
// template loses is thus absent in the test too, and the test fails where
// the page would.
//
// This file is NOT under frontend/html, thus frontend.Static does not embed
// it and no byte of it reaches a device.

'use strict';

const fs = require('fs');
const path = require('path');
const assert = require('node:assert');
const { newPage, run } = require('./page-stub.js');
const { newDocument, Event, EventTarget, NodeFilter, FormData } = require('./mini-dom.js');

const templateDir = path.join(__dirname, '..', 'templates');

// template answers the text of one template. values gives the text of a
// placeholder by its name, and each other placeholder is empty.
function template(name, values) {
    values = values || {};
    return fs.readFileSync(path.join(templateDir, name), 'utf8')
        .replace(/%%([A-Z0-9_]+)%%/g, function (all, key) {
            return Object.prototype.hasOwnProperty.call(values, key) ? values[key] : '';
        });
}

// shellBody answers the body of a note page: the body of index.html and
// the modals that the server adds when it serves the page. values fills
// the placeholders of index.html, for example PREVIEW_BODY.
function shellBody(values) {
    const index = template('index.html', values);
    const open = index.indexOf('<body>');
    const close = index.indexOf('</body>');
    assert.ok(open >= 0 && close > open, 'index.html has no body element');
    return index.slice(open + '<body>'.length, close) + template('modals.html');
}

// shellScripts answers the application scripts that index.html names, in
// its order. omn-go-compat.js is not in the list, because it is the notice
// of an old browser and gives the page nothing. The vendored libraries and
// the user file omn-go-custom.js are not in the list either.
function shellScripts() {
    const src = fs.readFileSync(path.join(templateDir, 'index.html'), 'utf8');
    const out = [];
    const re = /js\/OMN-Go\/(omn-go-[a-z]+\.js)/g;
    let m;
    while ((m = re.exec(src)) !== null) {
        if (m[1] !== 'omn-go-compat.js') out.push(m[1]);
    }
    return out;
}

// newDomPage answers a page that is ready for a test.
//
// opts.body is the markup of the body. The default is shellBody().
// opts.preview is the markup of the note, inside #preview.
// opts.protocol is "http:" or "file:". The default is "http:".
// opts.note is the name of the note, with no extension. The default is
// "Note". The page gets the variables that the script block of index.html
// sets: PageName, currentNote, Title, PAGE_EXT and IS_MARKDOWN.
// opts.markdown is false for a page that is a file and not a note.
// opts.path is the path of the page. The default comes from opts.note.
// opts.meta gives the meta elements of the head, as {name: content}. The
// server writes one for each line of the header block of a note.
//
// The answer holds:
//
//	page       The window, which is also the global object of each script.
//	document   The document of mini-dom.js.
//	requests   Each call of fetch: {url, method, body}.
//	dialogs    Each alert, confirm and prompt: {kind, text}.
//	loads      The src of each script element that a script added.
//	streams    Each EventSource that a script opened.
//	went       Each address that a script opened in place of this page.
//	opened     Each call of window.open: {url, target}.
//	clipboard  Each text that a script wrote with the Clipboard API.
//	timers     The timers of the page, after h.holdTimers().
//
// A test sets h.server to answer a request, and h.confirm and h.prompt to
// answer a dialog.
function newDomPage(opts) {
    opts = opts || {};
    const page = newPage();
    const doc = newDocument(opts.body === undefined
        ? shellBody({ PREVIEW_BODY: opts.preview || '' }) : opts.body);
    const win = new EventTarget();
    doc.defaultView = win;
    doc.readyState = 'loading';
    for (const name of Object.keys(opts.meta || {})) {
        const meta = doc.createElement('meta');
        meta.setAttribute('name', name);
        meta.setAttribute('content', opts.meta[name]);
        doc.head.appendChild(meta);
    }

    page.document = doc;
    page.NodeFilter = NodeFilter;
    page.Event = Event;
    page.addEventListener = win.addEventListener.bind(win);
    page.removeEventListener = win.removeEventListener.bind(win);
    page.dispatchEvent = win.dispatchEvent.bind(win);
    page.location.protocol = opts.protocol || 'http:';
    const note = opts.note || 'Note';
    page.PageName = note;
    page.currentNote = note;
    page.Title = opts.title || note;
    page.PAGE_EXT = '.md';
    page.IS_MARKDOWN = opts.markdown !== false;
    page.location.pathname = opts.path || '/' + note + '.html';
    page.location.reloads = 0;
    page.location.reload = function () { page.location.reloads++; };
    // A script that sets location.href leaves the page. The document of a
    // test stays, thus href keeps the address of this page, and h.went
    // gets each address that a script tried to open.
    Object.defineProperty(page.location, 'href', {
        get() {
            return page.location.origin + page.location.pathname +
                page.location.search + page.location.hash;
        },
        set(value) { h.went.push(String(value)); },
    });
    page.location.replace = function (url) { h.went.push(String(url)); };
    page.location.assign = function (url) { h.went.push(String(url)); };
    // A new hash sends hashchange, the same as in a browser.
    let hash = '';
    Object.defineProperty(page.location, 'hash', {
        get() { return hash; },
        set(value) {
            const next = value && value[0] !== '#' ? '#' + value : (value || '');
            if (next === hash) return;
            hash = next;
            win.dispatchEvent(new Event('hashchange'));
        },
    });
    page.FormData = FormData;
    page.scrollTo = function () {};
    // window.open keeps each address, thus a test can read h.opened.
    page.open = function (url, target) { h.opened.push({ url: String(url), target: target }); };
    // The clipboard of the browser. A test reads h.clipboard, and it can
    // set h.clipboardRefuses to model a browser that refuses the write.
    page.isSecureContext = true;
    page.navigator.clipboard = {
        writeText: async function (text) {
            if (h.clipboardRefuses) throw new Error('NotAllowedError');
            h.clipboard.push(String(text));
        },
    };
    page.history.backs = 0;
    page.history.back = function () { page.history.backs++; };

    // A console of its own. omn-go-console.js puts a hook on each console
    // method, and the console of Node is one object for all the tests.
    page.console = { log() {}, info() {}, warn() {}, error() {}, debug() {}, trace() {}, table() {}, dir() {}, time() {}, timeEnd() {} };

    const h = {
        page: page, document: doc, requests: [], dialogs: [], loads: [], streams: [],
        opened: [], clipboard: [], timers: [], went: [],
        // server answers one request. A test replaces it. The default is
        // a JSON success with no data.
        server: function () { return { status: 'success' }; },
        confirm: function () { return false; },
        prompt: function () { return null; },
    };

    // fetch gives the request to h.server. The answer of h.server is the
    // JSON body, or {httpStatus, json, text} for another status, or an
    // Error for a request that the network refused.
    page.fetch = async function (url, init) {
        init = init || {};
        // The body is text. URLSearchParams and FormData both give the
        // form of a query.
        const request = {
            url: String(url),
            method: init.method || 'GET',
            body: init.body === undefined ? null : String(init.body),
        };
        h.requests.push(request);
        const answer = await h.server(request);
        if (answer instanceof Error) throw answer;
        const full = answer && answer.httpStatus !== undefined ? answer : { httpStatus: 200, json: answer };
        return {
            ok: full.httpStatus >= 200 && full.httpStatus < 300,
            status: full.httpStatus,
            json: async function () {
                if (full.json === undefined) throw new Error('the answer is not JSON');
                return full.json;
            },
            text: async function () {
                return full.text !== undefined ? full.text : JSON.stringify(full.json);
            },
        };
    };
    // The log stream of the server. A test sends a line with
    // h.serverLog(text).
    page.EventSource = function (url) {
        this.url = url;
        this.close = function () {};
        this.addEventListener = function () {};
        h.streams.push(this);
    };
    h.serverLog = function (text) {
        for (const s of h.streams) {
            if (typeof s.onmessage === 'function') s.onmessage({ data: text });
        }
    };
    page.alert = function (text) { h.dialogs.push({ kind: 'alert', text: String(text) }); };
    page.confirm = function (text) {
        h.dialogs.push({ kind: 'confirm', text: String(text) });
        return h.confirm(String(text));
    };
    page.prompt = function (text, value) {
        h.dialogs.push({ kind: 'prompt', text: String(text) });
        return h.prompt(String(text), value);
    };

    // A script element that a script adds to the head is a lazy file. The
    // page runs that file and then calls onload, the same as a browser.
    const appendToHead = doc.head.appendChild.bind(doc.head);
    doc.head.appendChild = function (el) {
        appendToHead(el);
        if (el.tagName === 'SCRIPT' && el.getAttribute('src')) {
            const src = el.getAttribute('src');
            h.loads.push(src);
            setTimeout(function () {
                const err = run(page, path.basename(src));
                if (err) {
                    if (typeof el.onerror === 'function') el.onerror(err);
                    throw err;
                }
                if (typeof el.onload === 'function') el.onload();
            }, 0);
        }
        return el;
    };

    // load runs each script in order. A script that throws fails the test.
    h.load = function (files) {
        for (const file of files) {
            const err = run(page, file);
            assert.strictEqual(err, null, file + ' did not load: ' + (err && err.stack));
        }
        return h;
    };
    // ready ends the load of the page: DOMContentLoaded on the document,
    // and then load on the window.
    h.ready = function () {
        doc.readyState = 'interactive';
        doc.dispatchEvent(new Event('DOMContentLoaded', { bubbles: true }));
        doc.readyState = 'complete';
        win.dispatchEvent(new Event('load'));
        return h;
    };
    // $ answers the one element of a selector. An absent element fails
    // the test with the selector in the message.
    h.$ = function (selector) {
        const el = doc.querySelector(selector);
        assert.ok(el, 'the page has no element for ' + selector);
        return el;
    };
    // type puts a value into a box and sends the input event, the same as
    // a key press does.
    h.type = function (selector, value) {
        const el = h.$(selector);
        el.value = value;
        el.dispatchEvent(new Event('input', { bubbles: true }));
        return h;
    };
    // key sends a keydown to the one element of a selector and answers the
    // event. init can hold ctrlKey and the other fields of a key event.
    h.key = function (selector, name, init) {
        const event = new Event('keydown', Object.assign({ bubbles: true, key: name }, init || {}));
        h.$(selector).dispatchEvent(event);
        return event;
    };
    // press sends a click to the one element of a selector.
    h.press = function (selector) {
        h.$(selector).click();
        return h;
    };
    // settle waits until each promise and each short timer of the page
    // ended. rounds is the count of timer turns, and the default is 5.
    h.settle = async function (rounds) {
        for (let i = 0; i < (rounds || 5); i++) {
            await new Promise(function (resolve) { setTimeout(resolve, 0); });
        }
    };
    // holdTimers takes the timers of the page away from the clock. Each
    // setTimeout of a script goes to h.timers as {fn, ms, cleared}, and
    // none runs until the test calls runTimers. A test of a wait of some
    // seconds then ends at once.
    h.holdTimers = function () {
        page.setTimeout = function (fn, ms) {
            const timer = { fn: fn, ms: ms || 0, cleared: false, ran: false };
            h.timers.push(timer);
            return timer;
        };
        page.clearTimeout = function (timer) {
            if (timer && typeof timer === 'object') timer.cleared = true;
        };
        return h;
    };
    // pendingTimers answers the wait of each timer that can still run.
    h.pendingTimers = function () {
        return h.timers.filter(function (t) { return !t.cleared && !t.ran; })
            .map(function (t) { return t.ms; });
    };
    // runTimers runs each timer that can still run.
    h.runTimers = function () {
        for (const t of h.timers.slice()) {
            if (t.cleared || t.ran) continue;
            t.ran = true;
            t.fn();
        }
        return h;
    };
    // hidden tells whether the class "hidden" or the style hides an
    // element.
    h.hidden = function (selector) {
        const el = h.$(selector);
        return el.classList.contains('hidden') || el.style.display === 'none';
    };
    return h;
}

// notePage answers a page with the scripts of index.html, after the load
// events. It is what a person has in front of them when they press a
// control of a note page.
function notePage(opts) {
    return newDomPage(opts).load(shellScripts()).ready();
}

// configPage answers the Config page after the load events. The page has
// two git server cards, with the slot numbers 0 and 1. values fills the
// placeholders of config_page.html.
function configPage(values) {
    const cards = [0, 1].map(function (i) {
        return template('git_server_card.html', { INDEX: String(i), SLOT: String(i + 1) });
    }).join('');
    const body = shellBody() + template('config_page.html', Object.assign({ GIT_SERVERS: cards }, values || {}));
    return newDomPage({ body: body, path: '/Config.html' })
        .load(shellScripts().concat(['omn-go-config.js'])).ready();
}

// editorPage answers the editor page with text as the note, after the load
// events and the load of the note. The editor is a page of its own: it has
// the markup of editor.html and the script omn-go-editor.js, and no other
// script of the application.
async function editorPage(text) {
    const html = template('editor.html');
    const open = html.indexOf('<body');
    const start = html.indexOf('>', open) + 1;
    const close = html.indexOf('</body>');
    assert.ok(open >= 0 && close > start, 'editor.html has no body element');
    const h = newDomPage({ body: html.slice(start, close), path: '/Note.md' });
    h.page.OMN_EDIT_NAME = 'Note';
    h.page.OMN_EDIT_EXT = '.md';
    h.page.OMN_EDIT_VIEW = '/Note.html';
    h.page.getComputedStyle = function () { return { lineHeight: '18px', fontSize: '14px' }; };
    h.server = function (request) {
        if (request.url.indexOf('/api/note') === 0 && request.method === 'GET') {
            return { httpStatus: 200, text: text };
        }
        return { status: 'success' };
    };
    h.load(['omn-go-editor.js']).ready();
    await h.settle();
    return h;
}

module.exports = { newDomPage, notePage, configPage, editorPage, shellBody, shellScripts, template };
