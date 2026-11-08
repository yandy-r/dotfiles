// tmux-style prefix, matching ~/.config/herdr/config.toml ([keys], prefix ctrl+b).
// keys.json binds ctrl-b to `mod.open:prefix`; this popup takes the next key,
// runs its command and closes. Esc, or any unbound key, cancels.
// Pressing the prefix again sends a literal ctrl+b to the focused terminal.

import { View, div } from 'gpui-kit';
import { v_flex, h_flex } from 'gpui-base';
import { run, props, closeOverlay, theme, focused, hc, openOverlay } from 'herdr';

const PREFIX = 'ctrl-b';

// [keystrokes, label, command, args]
const BINDINGS = [
  [['c'], 'new tab', 'tab.new'],
  [['n'], 'next tab', 'tab.next'],
  [['p'], 'previous tab', 'tab.prev'],
  [['1-9'], 'go to tab', null],
  [['h'], 'pane left', 'pane.go', { direction: 'left' }],
  [['j'], 'pane down', 'pane.go', { direction: 'down' }],
  [['k'], 'pane up', 'pane.go', { direction: 'up' }],
  [['l'], 'pane right', 'pane.go', { direction: 'right' }],
  [['v', '|', 'shift-|', 'shift-\\'], 'split right', 'pane.split', { direction: 'right' }],
  [['s', '-', 'minus'], 'split down', 'pane.split', { direction: 'down' }],
  [['x'], 'close pane', 'pane.close'],
  [['shift-x', 'X'], 'close tab', 'tab.close'],
  [['z'], 'zoom pane', 'pane.zoom'],
  [['g'], 'go to…', 'mod.open', { name: 'picker' }],
  [['t'], 'jump to anything', 'mod.open', { name: 'picker' }],
  [['shift-b', 'B'], 'toggle sidebar', 'layout.toggle', { id: 'sidebar' }],
  [['shift-s', 'S'], 'settings', 'app.settings'],
  [['d', 'q'], 'close window', 'window.close'],
  // Broadcast presets (mods/broadcast): the dialog can change them before sending.
  [['a'], 'broadcast: tab', '@broadcast', { scope: 'tab' }],
  [['shift-a', 'A'], 'broadcast: space', '@broadcast', { scope: 'space' }],
  [['e'], 'broadcast: all, local', '@broadcast', { scope: 'all', machines: 'local' }],
  [['shift-r', 'R'], 'broadcast: all, remote', '@broadcast', { scope: 'all', machines: 'remote' }],
  [['shift-e', 'E'], 'broadcast: everywhere', '@broadcast', { scope: 'all', machines: 'all' }],
];

// "@broadcast" opens the broadcast dialog with the binding's args as presets.
function act(command, args) {
  if (command === '@broadcast') {
    openOverlay({ mod: 'broadcast', props: args, kind: 'dialog', width: 620, height: 520 });
  } else {
    run(command, args);
  }
}

function lookup(keystroke) {
  if (/^[1-9]$/.test(keystroke)) return ['tab.select', { index: Number(keystroke) }];
  for (const [keys, , command, args] of BINDINGS) {
    if (command && keys.includes(keystroke)) return [command, args ?? {}];
  }
  return null;
}

export default class Prefix extends View {
  init(_props, cx) {
    this.target = focused()?.terminal ?? null;
    this.handle = cx.focus_handle();
    this.handle.focus();
  }

  close() {
    closeOverlay(props().overlay);
  }

  press(e) {
    const ks = e.keystroke;
    if (ks === 'escape') return this.close();
    if (ks === PREFIX) {
      this.close();
      if (this.target) hc(['keys', this.target, 'ctrl+b']);
      return;
    }
    // Shifted punctuation may arrive as "shift-|" or as the bare character.
    const hit = lookup(ks) ?? (e.key_char ? lookup(e.key_char) : null);
    if (!hit && e.modifiers?.shift) return; // a lone shift press: keep waiting
    this.close();
    if (hit) act(hit[0], hit[1]);
  }

  render(cx) {
    const t = theme();
    const row = ([keys, label]) =>
      h_flex()
        .gap(8)
        .w(170)
        .child(div().w(52).text_color(t.accent).font_semibold().child(keys[0]))
        .child(div().text_color(t.muted).child(label));
    return v_flex()
      .id('prefix')
      .track_focus(this.handle)
      .on_key_down((e, cx) => {
        this.press(e);
        cx.stop_propagation();
      })
      .size_full()
      .p(14)
      .gap(10)
      .bg(t.popover)
      .text_size(12)
      .child(
        h_flex()
          .items_center()
          .child(div().flex_1().text_size(13).font_semibold().text_color(t.foreground).child('ctrl+b'))
          .child(div().text_size(11).text_color(t.dim).child('Esc cancels · ctrl+b sends ctrl+b'))
      )
      .child(h_flex().flex_wrap().gap_y(4).children(BINDINGS.map(row)));
  }
}
