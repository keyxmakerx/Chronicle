/**
 * TipTap Bundle Source
 *
 * This file is compiled by esbuild into tiptap-bundle.min.js.
 * Run: npx esbuild static/vendor/tiptap-bundle.src.js --bundle --minify --outfile=static/vendor/tiptap-bundle.min.js --format=iife --global-name=__TipTapInternal
 *
 * Or use: make tiptap-bundle
 */

import { Editor, Node, Mark, Extension, mergeAttributes } from '@tiptap/core';
import StarterKit from '@tiptap/starter-kit';
import Placeholder from '@tiptap/extension-placeholder';
import Link from '@tiptap/extension-link';
import Underline from '@tiptap/extension-underline';
import { TaskList, TaskItem } from '@tiptap/extension-list';
import { Table } from '@tiptap/extension-table';
import { TableRow } from '@tiptap/extension-table-row';
import { TableCell } from '@tiptap/extension-table-cell';
import { TableHeader } from '@tiptap/extension-table-header';
import CodeBlockLowlight from '@tiptap/extension-code-block-lowlight';
import { common, createLowlight } from 'lowlight';

// Create lowlight instance with common languages (JS, Python, HTML, CSS,
// JSON, SQL, Bash, Ruby, Go, Java, C, C++, TypeScript, Markdown, YAML, XML).
var lowlight = createLowlight(common);

// Expose on window.TipTap for use by Chronicle widgets. Node/Mark/Extension
// and mergeAttributes let a widget define its own schema types (the Journal's
// [[note]] link node) instead of bending an unrelated built-in one.
window.TipTap = {
  Editor,
  Node,
  Mark,
  Extension,
  mergeAttributes,
  StarterKit,
  Placeholder,
  Link,
  Underline,
  TaskList,
  TaskItem,
  Table,
  TableRow,
  TableCell,
  TableHeader,
  CodeBlockLowlight,
  lowlight,
};
