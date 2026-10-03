// The bookmark panel and the share from Android, in the REAL JavaScript and
// on the markup of the templates.
//
// MainActivity.java calls window.handleShare with the text that another
// Android app shares. That text has no fixed form. The function must find
// the address in it, and it must not lose the text when it finds none.
//
// The Tags box offers the tags that the Bookmarks page already holds. A
// person types a part of a tag, and the list must not offer a tag that the
// box already has.

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const { notePage } = require('./dom-page.js');
const { Event } = require('./mini-dom.js');

const TAGS = ['golang', 'git', 'Go-Modules', 'android', 'notes'];

// tagServer answers the tag list of the Bookmarks page and a success for
// each other request.
function tagServer(request) {
    if (request.url === '/json/bookmarker-tags.json') return TAGS;
    return { status: 'success' };
}

// type puts a value into a box and sends the input event, the same as a
// key press does.
function type(h, selector, value) {
    const el = h.$(selector);
    el.value = value;
    el.dispatchEvent(new Event('input', { bubbles: true }));
}

function key(h, selector, name) {
    const event = new Event('keydown', { bubbles: true, key: name });
    h.$(selector).dispatchEvent(event);
    return event;
}

function suggestions(h) {
    return h.document.querySelectorAll('#bmTagsSuggestions .tag-suggestion-item')
        .map(function (li) { return li.textContent; });
}

test('the bookmark button loads its file and opens and closes the panel', async () => {
    const h = notePage();
    h.server = tagServer;
    assert.strictEqual(h.hidden('#bmPanel'), true);
    h.press('[data-action="bookmark-panel"]');
    await h.settle();
    assert.deepStrictEqual(h.loads, ['/js/OMN-Go/omn-go-bookmark.js']);
    assert.strictEqual(h.hidden('#bmPanel'), false);
    h.press('[data-action="bookmark-panel"]');
    await h.settle();
    assert.strictEqual(h.hidden('#bmPanel'), true);
    assert.strictEqual(h.loads.length, 1);
});

test('the panel asks for the tags one time', async () => {
    const h = notePage();
    h.server = tagServer;
    h.press('[data-action="bookmark-panel"]');
    await h.settle();
    h.press('[data-action="bookmark-panel"]');
    h.press('[data-action="bookmark-panel"]');
    await h.settle();
    const asks = h.requests.filter(function (r) { return r.url === '/json/bookmarker-tags.json'; });
    assert.strictEqual(asks.length, 1);
});

test('the Tags box offers the tags that start with the typed part', async () => {
    const h = notePage();
    h.server = tagServer;
    h.press('[data-action="bookmark-panel"]');
    await h.settle();

    // One character is below the limit of the box, which is minChars="2".
    type(h, '#bmTags', 'g');
    await h.settle();
    assert.deepStrictEqual(suggestions(h), []);
    assert.strictEqual(h.hidden('#bmTagsSuggestions'), true);

    // The case of the letters does not matter.
    type(h, '#bmTags', 'go');
    await h.settle();
    assert.deepStrictEqual(suggestions(h), ['golang', 'Go-Modules']);
    assert.strictEqual(h.hidden('#bmTagsSuggestions'), false);

    // Only the part after the last comma counts, and a tag that the box
    // already holds is not in the list.
    type(h, '#bmTags', 'golang, android, g');
    await h.settle();
    assert.deepStrictEqual(suggestions(h), []);
    type(h, '#bmTags', 'golang, android, gi');
    await h.settle();
    assert.deepStrictEqual(suggestions(h), ['git']);
    type(h, '#bmTags', 'GOLANG, go');
    await h.settle();
    assert.deepStrictEqual(suggestions(h), ['Go-Modules']);
});

test('the arrow keys and Enter put a tag into the box', async () => {
    const h = notePage();
    h.server = tagServer;
    h.press('[data-action="bookmark-panel"]');
    await h.settle();
    type(h, '#bmTags', 'notes, go');
    await h.settle();

    // Enter with no active line is not for the list.
    assert.strictEqual(key(h, '#bmTags', 'Enter').defaultPrevented, false);

    key(h, '#bmTags', 'ArrowDown');
    key(h, '#bmTags', 'ArrowDown');
    const active = h.document.querySelectorAll('#bmTagsSuggestions .active');
    assert.deepStrictEqual(active.map(function (li) { return li.textContent; }), ['Go-Modules']);
    // The list goes round at its end.
    key(h, '#bmTags', 'ArrowDown');
    assert.strictEqual(h.$('#bmTagsSuggestions .active').textContent, 'golang');
    key(h, '#bmTags', 'ArrowUp');
    assert.strictEqual(h.$('#bmTagsSuggestions .active').textContent, 'Go-Modules');

    const enter = key(h, '#bmTags', 'Enter');
    assert.strictEqual(enter.defaultPrevented, true, 'Enter must not send a form');
    assert.strictEqual(h.$('#bmTags').value, 'notes, Go-Modules, ');
    assert.strictEqual(h.hidden('#bmTagsSuggestions'), true);
    assert.strictEqual(h.document.activeElement, h.$('#bmTags'), 'the box lost the focus');
});

test('a press on a line of the list puts that tag into the box', async () => {
    const h = notePage();
    h.server = tagServer;
    h.press('[data-action="bookmark-panel"]');
    await h.settle();
    type(h, '#bmTags', 'an');
    await h.settle();
    const line = h.$('#bmTagsSuggestions .tag-suggestion-item');
    const down = new Event('mousedown', { bubbles: true });
    line.dispatchEvent(down);
    // mousedown and not click: the default of mousedown takes the focus
    // away from the box, and the list would close before the click.
    assert.strictEqual(down.defaultPrevented, true);
    assert.strictEqual(h.$('#bmTags').value, 'android, ');
});

test('Escape and a click outside close the list', async () => {
    const h = notePage();
    h.server = tagServer;
    h.press('[data-action="bookmark-panel"]');
    await h.settle();
    type(h, '#bmTags', 'go');
    await h.settle();
    key(h, '#bmTags', 'Escape');
    assert.strictEqual(h.hidden('#bmTagsSuggestions'), true);

    type(h, '#bmTags', 'go');
    await h.settle();
    assert.strictEqual(h.hidden('#bmTagsSuggestions'), false);
    h.$('#bmTitle').click();
    assert.strictEqual(h.hidden('#bmTagsSuggestions'), true);
});

test('a tag list that the server does not have gives an empty list and no fault', async () => {
    const h = notePage();
    h.server = function (request) {
        return request.url === '/json/bookmarker-tags.json' ? { httpStatus: 404, text: 'not found' } : {};
    };
    h.press('[data-action="bookmark-panel"]');
    await h.settle();
    type(h, '#bmTags', 'go');
    await h.settle();
    assert.deepStrictEqual(suggestions(h), []);
});

test('Save sends the four boxes, clears them and loads the page again', async () => {
    const h = notePage();
    h.server = tagServer;
    h.press('[data-action="bookmark-panel"]');
    await h.settle();
    h.$('#bmUrl').value = 'https://example.org/a?b=1&c=2';
    h.$('#bmTitle').value = 'An example';
    h.$('#bmTags').value = 'golang, notes';
    h.$('#bmNotes').value = 'first; second';
    h.press('[data-action="bookmark-save"]');
    await h.settle();
    const post = h.requests.filter(function (r) { return r.url === '/api/bookmark'; });
    assert.strictEqual(post.length, 1);
    assert.strictEqual(post[0].method, 'POST');
    assert.strictEqual(post[0].body,
        'url=https%3A%2F%2Fexample.org%2Fa%3Fb%3D1%26c%3D2&title=An+example&tags=golang%2C+notes&notes=first%3B+second');
    assert.strictEqual(h.hidden('#bmPanel'), true);
    for (const id of ['#bmUrl', '#bmTitle', '#bmTags', '#bmNotes']) {
        assert.strictEqual(h.$(id).value, '', id + ' keeps its text after the save');
    }
    assert.strictEqual(h.page.location.reloads, 1);
});

test('a save that the server refuses keeps the panel and its text', async () => {
    const h = notePage();
    h.server = function (request) {
        if (request.url === '/api/bookmark') return { httpStatus: 401, text: 'no' };
        return tagServer(request);
    };
    h.press('[data-action="bookmark-panel"]');
    await h.settle();
    h.$('#bmUrl').value = 'https://example.org/';
    h.press('[data-action="bookmark-save"]');
    await h.settle();
    assert.strictEqual(h.hidden('#bmPanel'), false);
    assert.strictEqual(h.$('#bmUrl').value, 'https://example.org/');
    assert.strictEqual(h.page.location.reloads, 0);
});

test('a shared text with an address opens the bookmark panel', async () => {
    const h = notePage();
    h.server = tagServer;
    // The stub of omnLazy answers with a promise. Android does not read it.
    await h.page.handleShare('Look at this https://example.org/page it is good', '');
    await h.settle();
    assert.strictEqual(h.$('#bmUrl').value, 'https://example.org/page');
    assert.strictEqual(h.$('#bmTitle').value, 'Look at this  it is good');
    assert.strictEqual(h.hidden('#bmPanel'), false);
    assert.strictEqual(h.hidden('#quickPanel'), true);

    // The subject is the title, when it is not the address itself.
    await h.page.handleShare('https://example.org/x', 'A page title');
    assert.strictEqual(h.$('#bmTitle').value, 'A page title');
    await h.page.handleShare('https://example.org/x', 'https://example.org/x');
    assert.strictEqual(h.$('#bmTitle').value, 'Shared Link');
});

test('a shared text with no address opens the Quick Note panel with the whole text', async () => {
    const h = notePage();
    h.server = tagServer;
    await h.page.handleShare('a line of text', 'The subject');
    await h.settle();
    assert.strictEqual(h.$('#quickText').value, 'The subject\n\na line of text');
    assert.strictEqual(h.hidden('#quickPanel'), false);
    assert.strictEqual(h.hidden('#bmPanel'), true);
});

test('omnGoInsertCapture answers true at once and opens the Quick Note panel', () => {
    const h = notePage();
    // IntentBridge compares the answer with true. A promise is not true.
    const answer = h.page.omnGoInsertCapture('4006381333931', 'EAN-13');
    assert.strictEqual(answer, true);
    assert.strictEqual(h.$('#quickText').value, 'EAN-13\n\n4006381333931');
    assert.strictEqual(h.hidden('#quickPanel'), false);
    assert.deepStrictEqual(h.loads, [], 'the capture loaded a lazy file');
});
