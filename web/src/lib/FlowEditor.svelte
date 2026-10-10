<script lang="ts">
	import { autocompletion, type CompletionContext } from '@codemirror/autocomplete';
	import { yaml } from '@codemirror/lang-yaml';
	import { linter, type Diagnostic } from '@codemirror/lint';
	import { EditorView, keymap } from '@codemirror/view';
	import { basicSetup } from 'codemirror';
	import { untrack } from 'svelte';
	import { checkFlow, offset } from './flows';
	import { complete, type Schema } from './yamlschema';

	type Props = {
		value: string;
		source: string;
		schema?: Schema;
		onsave?: () => void;
	};
	let { value = $bindable(), source, schema, onsave }: Props = $props();

	function completions(ctx: CompletionContext) {
		if (!schema) return null;
		const line = ctx.state.doc.lineAt(ctx.pos);
		const r = complete(schema, ctx.state.doc.toString(), ctx.pos);
		if (!r || r.options.length === 0) return null;
		const from = line.from + r.from;
		if (from === ctx.pos && !ctx.explicit && r.kind === 'key') return null;
		return {
			from,
			options: r.options.map((o) => ({
				label: o.label,
				detail: o.detail,
				info: o.info,
				apply: o.apply,
				type: r.kind === 'key' ? 'property' : 'enum'
			})),
			validFor: /^[\w.-]*$/
		};
	}

	// Server-side validation: same checks as when the file is loaded.
	async function lint(view: EditorView): Promise<Diagnostic[]> {
		const text = view.state.doc.toString();
		try {
			const problems = await checkFlow(source || 'new.yaml', text);
			return problems.map((p) => {
				const from = offset(text, p.line, p.col);
				const end = view.state.doc.lineAt(from).to;
				return { from, to: Math.max(end, from), severity: 'error', message: p.message };
			});
		} catch (e) {
			const message = e instanceof Error ? e.message : String(e);
			return [{ from: 0, to: 0, severity: 'warning', message: `Validation failed: ${message}` }];
		}
	}

	const theme = EditorView.theme({
		'&': { color: 'inherit', backgroundColor: 'transparent', fontSize: '0.875rem' },
		'.cm-content': { caretColor: 'currentColor' },
		'.cm-cursor': { borderLeftColor: 'currentColor' },
		'.cm-gutters': {
			backgroundColor: 'transparent',
			color: 'inherit',
			opacity: 0.6,
			borderRight: '1px solid color-mix(in srgb, currentColor 15%, transparent)'
		},
		'.cm-activeLine, .cm-activeLineGutter': {
			backgroundColor: 'color-mix(in srgb, currentColor 6%, transparent)'
		},
		'&.cm-focused .cm-selectionBackground, .cm-selectionBackground': {
			backgroundColor: 'color-mix(in srgb, Highlight 40%, transparent)'
		}
	});

	// Created once; later prop changes do not reset the document (use {#key} to reload).
	function editor(el: HTMLElement) {
		const view = new EditorView({
			parent: el,
			doc: untrack(() => value),
			extensions: [
				keymap.of([
					{
						key: 'Mod-s',
						preventDefault: true,
						run: () => {
							onsave?.();
							return true;
						}
					}
				]),
				basicSetup,
				yaml(),
				theme,
				autocompletion({ override: [completions] }),
				linter(lint, { delay: 500 }),
				EditorView.updateListener.of((u) => {
					if (u.docChanged) value = u.state.doc.toString();
				})
			]
		});
		return () => view.destroy();
	}
</script>

<div class="editor" {@attach editor}></div>

<style>
	.editor {
		border: 1px solid color-mix(in srgb, currentColor 20%, transparent);
		border-radius: 0.25rem;
	}
	.editor :global(.cm-editor) {
		min-height: 24rem;
		max-height: 75vh;
	}
	.editor :global(.cm-scroller) {
		font-family: ui-monospace, monospace;
	}
</style>
