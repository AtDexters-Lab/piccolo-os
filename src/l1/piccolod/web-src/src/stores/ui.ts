import { writable } from 'svelte/store';

type Toast = { id: number; message: string; type: 'info'|'success'|'error' };

const _toasts = writable<Toast[]>([]);
let counter = 0;

export const toasts = _toasts;

export function toast(message: string, type: Toast['type'] = 'info') {
  const id = ++counter;
  _toasts.update(arr => [...arr, { id, message, type }]);
  return id;
}

export function removeToast(id: number) {
  _toasts.update(arr => arr.filter(t => t.id !== id));
}

