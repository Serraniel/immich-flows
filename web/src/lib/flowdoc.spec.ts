import { describe, expect, it } from 'vitest';
import { data, insertStep, move, parse, removeAt, set, setPosition, stepAt, text } from './flowdoc';
import { COL, graph, ROW } from './flowgraph';

const src = `# my flow
id: a
trigger:
  on: asset.uploaded # comment kept
steps:
  - id: h
    use: hash
  - use: switch
    cases:
      - if: 'x'
        steps:
          - use: log
            with: { message: a }
          - use: log
            with: { message: b }
      - steps:
          - use: tag.assign
  - use: log
    with: { message: end }
`;

describe('flowdoc', () => {
	it('edits fields keeping comments', () => {
		const doc = parse(src);
		set(doc, ['steps', 0, 'with', 'algorithms'], ['md5']);
		set(doc, ['trigger', 'if'], 'asset.type == "IMAGE"');
		set(doc, ['steps', 0, 'id'], '');
		const out = doc.toString();
		expect(out).toContain('# my flow');
		expect(out).toContain('on: asset.uploaded # comment kept');
		expect(data(parse(out)).steps![0]).toEqual({ use: 'hash', with: { algorithms: ['md5'] } });
		expect(data(doc).trigger!.if).toBe('asset.type == "IMAGE"');
	});

	it('inserts, removes and moves steps, keeping positions attached', () => {
		const doc = parse(src);
		setPosition(doc, 'steps.0', { x: 1, y: 2 });
		setPosition(doc, 'steps.2', { x: 3, y: 4 });
		setPosition(doc, 'steps.1.cases.0.steps.0', { x: 5, y: 6 });

		insertStep(doc, ['steps'], 0, { use: 'set' });
		let d = data(doc);
		expect(d.steps!.map((s) => s.use)).toEqual(['set', 'hash', 'switch', 'log']);
		expect(d.ui!.positions).toEqual({
			'steps.1': { x: 1, y: 2 },
			'steps.3': { x: 3, y: 4 },
			'steps.2.cases.0.steps.0': { x: 5, y: 6 }
		});

		move(doc, ['steps', 3], -2);
		d = data(doc);
		expect(d.steps!.map((s) => s.use)).toEqual(['set', 'log', 'hash', 'switch']);
		expect(d.ui!.positions).toEqual({
			'steps.2': { x: 1, y: 2 },
			'steps.1': { x: 3, y: 4 },
			'steps.3.cases.0.steps.0': { x: 5, y: 6 }
		});

		removeAt(doc, ['steps', 1]);
		d = data(doc);
		expect(d.steps!.map((s) => s.use)).toEqual(['set', 'hash', 'switch']);
		expect(Object.keys(d.ui!.positions!).sort()).toEqual(['steps.1', 'steps.2.cases.0.steps.0']);

		insertStep(doc, ['steps', 2, 'cases', 1, 'steps'], 5, { use: 'log' });
		expect(data(doc).steps![2].cases![1].steps!.map((s) => s.use)).toEqual(['tag.assign', 'log']);
		expect(doc.toString()).toContain('# my flow');
	});

	it('keeps flow style when replacing a collection', () => {
		const doc = parse('steps:\n  - use: hash\n    with:\n      algorithms: [md5, sha1]\n');
		set(doc, ['steps', 0, 'with', 'algorithms'], ['md5']);
		expect(text(doc)).toContain('algorithms: [md5]');
	});

	it('creates missing lists', () => {
		const doc = parse('id: a\ntrigger: {on: manual}\n');
		insertStep(doc, ['steps'], 0, { use: 'log' });
		expect(data(doc).steps).toEqual([{ use: 'log' }]);
	});

	it('maps offsets to steps', () => {
		const doc = parse(src);
		expect(stepAt(doc, src.indexOf('asset.uploaded'))).toBe('trigger');
		expect(stepAt(doc, src.indexOf('use: hash'))).toBe('steps.0');
		expect(stepAt(doc, src.indexOf('message: b'))).toBe('steps.1.cases.0.steps.1');
		expect(stepAt(doc, src.indexOf('message: end'))).toBe('steps.2');
		expect(stepAt(doc, 0)).toBeUndefined();
	});
});

describe('flowgraph', () => {
	it('lays out branches and joins them', () => {
		const { nodes, edges } = graph(data(parse(src)));
		const at = (id: string) => nodes.find((n) => n.id === id)!;
		expect(nodes.map((n) => n.id)).toEqual([
			'trigger',
			'steps.0',
			'steps.1',
			'steps.1.cases.0',
			'steps.1.cases.0.steps.0',
			'steps.1.cases.0.steps.1',
			'steps.1.cases.1',
			'steps.1.cases.1.steps.0',
			'steps.2'
		]);
		expect(at('steps.1').kind).toBe('switch');
		expect(at('steps.1.cases.1').label).toBe('default');
		expect(at('steps.1.cases.1').position).toEqual({ x: COL, y: 3 * ROW });
		// After the switch: below the longest branch, joined from both branch ends.
		expect(at('steps.2').position.y).toBe(6 * ROW);
		expect(edges.filter((e) => e.target === 'steps.2').map((e) => e.source)).toEqual([
			'steps.1.cases.0.steps.1',
			'steps.1.cases.1.steps.0'
		]);
	});

	it('uses saved positions', () => {
		const d = data(parse('id: a\nsteps: [{use: log}]\nui: {positions: {steps.0: {x: 7, y: 8}}}'));
		expect(graph(d).nodes[1].position).toEqual({ x: 7, y: 8 });
	});
});
