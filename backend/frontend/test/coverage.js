// The line coverage of the shipped scripts, from the coverage files of V8.
//
// Run the tests with NODE_V8_COVERAGE=<dir>. Each node process then writes
// one JSON file into <dir>. This program reads the files and prints one
// JSON object. For each application script it gives the count of code
// lines, the count of the lines that a test ran, and the share in percent.
//
//	NODE_V8_COVERAGE=/tmp/cov node --test backend/frontend/test/*.test.js
//	node backend/frontend/test/coverage.js /tmp/cov
//
// WHY NOT --experimental-test-coverage. The report of that option has a
// different form in each version of node, and an old node does not have the
// option. NODE_V8_COVERAGE is older and its files have one form.
//
// WHAT COUNTS AS A LINE. An empty line and a line that holds only a comment
// are not code, and this program does not count them. The report of node
// counts them, thus its numbers are higher.
//
// WHEN A LINE IS RUN. V8 writes ranges of characters with a count. A line is
// in the smallest range that holds the whole line. The line is run when the
// count of that range is above zero in one process or more.
//
// This file is NOT under frontend/html, thus frontend.Static does not embed
// it and no byte of it reaches a device.

'use strict';

const fs = require('fs');
const path = require('path');

const scriptDir = path.join(__dirname, '..', 'html', 'js', 'OMN-Go');

// shippedScripts answers the name of each application script. A vendored
// library has ".min." in its name and is not in the list.
function shippedScripts() {
    return fs.readdirSync(scriptDir)
        .filter(function (name) { return name.endsWith('.js') && name.indexOf('.min.') < 0; })
        .sort();
}

// codeLines answers, for each line of src, its start offset, its end offset
// and whether it holds code.
function codeLines(src) {
    const out = [];
    let offset = 0;
    let inBlock = false;
    for (const text of src.split('\n')) {
        let rest = text.trim();
        let code = false;
        while (rest.length > 0) {
            if (inBlock) {
                const end = rest.indexOf('*/');
                if (end < 0) { rest = ''; break; }
                inBlock = false;
                rest = rest.slice(end + 2).trim();
                continue;
            }
            if (rest.startsWith('//')) { rest = ''; break; }
            if (rest.startsWith('/*')) { inBlock = true; rest = rest.slice(2); continue; }
            code = true;
            // A block comment can start behind the code of this line.
            const open = rest.lastIndexOf('/*');
            if (open >= 0 && rest.indexOf('*/', open) < 0 && !/['"`]/.test(rest.slice(0, open))) inBlock = true;
            break;
        }
        // The offsets are of the text with no indent, the same as c8 uses.
        const lead = text.length - text.trimStart().length;
        out.push({ start: offset + lead, end: offset + text.trimEnd().length, code: code });
        offset += text.length + 1;
    }
    return out;
}

// runLines marks each line that the ranges of one process ran.
function runLines(lines, functions, hit) {
    // The smallest range that holds a line decides. Sort from large to
    // small, thus a later range replaces an earlier one.
    const ranges = [];
    for (const fn of functions) for (const r of fn.ranges) ranges.push(r);
    ranges.sort(function (a, b) { return (b.endOffset - b.startOffset) - (a.endOffset - a.startOffset); });
    const count = new Array(lines.length).fill(-1);
    for (const r of ranges) {
        for (let i = 0; i < lines.length; i++) {
            if (lines[i].start >= r.startOffset && lines[i].end <= r.endOffset) count[i] = r.count;
        }
    }
    for (let i = 0; i < lines.length; i++) if (count[i] > 0) hit[i] = true;
}

function main(dir) {
    const scripts = shippedScripts();
    const state = {};
    for (const name of scripts) {
        const lines = codeLines(fs.readFileSync(path.join(scriptDir, name), 'utf8'));
        state[name] = { lines: lines, hit: new Array(lines.length).fill(false) };
    }
    for (const file of fs.readdirSync(dir)) {
        if (!file.endsWith('.json')) continue;
        let data;
        try {
            data = JSON.parse(fs.readFileSync(path.join(dir, file), 'utf8'));
        } catch (e) {
            continue; // a process that ended while it wrote its file
        }
        for (const entry of data.result || []) {
            const at = entry.url.lastIndexOf('/html/js/OMN-Go/');
            if (at < 0) continue;
            const name = entry.url.slice(at + '/html/js/OMN-Go/'.length);
            if (state[name]) runLines(state[name].lines, entry.functions, state[name].hit);
        }
    }
    const report = {};
    for (const name of scripts) {
        let total = 0, covered = 0;
        const s = state[name];
        for (let i = 0; i < s.lines.length; i++) {
            if (!s.lines[i].code) continue;
            total++;
            if (s.hit[i]) covered++;
        }
        report[name] = {
            lines: total,
            covered: covered,
            percent: total === 0 ? 100 : Math.floor(1000 * covered / total) / 10,
        };
    }
    process.stdout.write(JSON.stringify(report, null, 1) + '\n');
}

if (require.main === module) {
    if (!process.argv[2]) {
        process.stderr.write('usage: node coverage.js <directory of NODE_V8_COVERAGE>\n');
        process.exit(2);
    }
    main(process.argv[2]);
}

module.exports = { codeLines, shippedScripts };
