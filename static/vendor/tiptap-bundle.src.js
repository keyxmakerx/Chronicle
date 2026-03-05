/**
 * TipTap Bundle Source
 *
 * This file is compiled by esbuild into tiptap-bundle.min.js.
 * Run: npx esbuild static/vendor/tiptap-bundle.src.js --bundle --minify --outfile=static/vendor/tiptap-bundle.min.js --format=iife --global-name=__TipTapInternal
 *
 * Or use: make tiptap-bundle
 */

import { Editor } from '@tiptap/core';
import StarterKit from '@tiptap/starter-kit';
import Placeholder from '@tiptap/extension-placeholder';
import Link from '@tiptap/extension-link';
import Underline from '@tiptap/extension-underline';
import { Table } from '@tiptap/extension-table';
import { TableRow } from '@tiptap/extension-table-row';
import { TableCell } from '@tiptap/extension-table-cell';
import { TableHeader } from '@tiptap/extension-table-header';
import CodeBlockLowlight from '@tiptap/extension-code-block-lowlight';
import { common, createLowlight } from 'lowlight';

// Create lowlight instance with common languages (JS, Python, HTML, CSS,
// JSON, SQL, Bash, Ruby, Go, Java, C, C++, TypeScript, Markdown, YAML, XML).
var lowlight = createLowlight(common);

// Expose on window.TipTap for use by Chronicle widgets.
window.TipTap = {
  Editor,
  StarterKit,
  Placeholder,
  Link,
  Underline,
  Table,
  TableRow,
  TableCell,
  TableHeader,
  CodeBlockLowlight,
  lowlight,
};
