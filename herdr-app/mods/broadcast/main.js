// Broadcast: type one command into many terminals at once, like tmux's
// synchronize-panes. It opens as a dialog. Pick a scope (this tab, this space,
// every space) and which machines to include, then press Enter. Each target
// gets the text through `hc send`, which pastes it and presses Enter.
//
// Open it with `mod.open:broadcast`, which uses the last choices, or with
// openOverlay({ mod: "broadcast", props: { scope, machines } }) to preset them
// (the prefix mod does this). Props:
//   scope:    "tab" | "space" | "all"
//   machines: "local" (this computer) | "remote" (every other machine) | "all"
//
// Keys while it's open: Enter sends, Esc closes, Up/Down step through history,
// alt-1/2/3 pick the scope, alt-l/alt-r/alt-a pick local, remote or all machines,
// alt-g toggles skipping agents, alt-i toggles including the focused terminal.

import { View, div } from 'gpui-kit';
import { v_flex, h_flex, InputState, Input } from 'gpui-base';
import { closeOverlay, focused, hc, machines, notify, panes, props, setShared, shared, spaces, theme } from 'herdr';

const PREFS = 'broadcast.prefs';
const HISTORY = 'broadcast.history';
const HISTORY_MAX = 30;
const LOCAL = ''; // this computer's key in the machine set (its machine is null)

const SCOPES = [
  ['tab', 'This tab', 'alt-1'],
  ['space', 'This space', 'alt-2'],
  ['all', 'All spaces', 'alt-3'],
];

const machineKey = (m) => m ?? LOCAL;
const machineLabel = (k) => (k === LOCAL ? 'this computer' : k);

// Every machine Herdr shows: this computer first, then the others by name.
function knownMachines() {
  const names = new Set([LOCAL]);
  for (const m of machines()) names.add(machineKey(m.name));
  for (const s of spaces()) names.add(machineKey(s.machine));
  return [...names].sort((a, b) => (a === LOCAL ? -1 : b === LOCAL ? 1 : a.localeCompare(b)));
}

// Turns a preset name, or a list of machine keys, into the set of machine keys to include.
function machineSet(preset) {
  const all = knownMachines();
  if (Array.isArray(preset)) return new Set(preset.filter((k) => all.includes(k)));
  if (preset === 'local') return new Set([LOCAL]);
  if (preset === 'remote') return new Set(all.filter((k) => k !== LOCAL));
  return new Set(all);
}

export default class Broadcast extends View {
  init(_props, cx) {
    const p = props() ?? {};
    const saved = shared(PREFS) ?? {};
    this.scope = p.scope ?? saved.scope ?? 'space';
    this.machines = machineSet(p.machines ?? saved.machines ?? 'local');
    this.skipAgents = saved.skipAgents ?? true;
    this.includeSelf = saved.includeSelf ?? true;
    this.history = Array.isArray(shared(HISTORY)) ? shared(HISTORY) : [];
    this.historyAt = -1; // -1: the line being typed, 0: the newest history entry
    this.draft = '';
    this.sending = false;

    // Remember where the dialog was opened from: focus moves while it's open.
    const f = focused();
    this.origin = { space: f?.space ?? null, tab: f?.tab ?? null, terminal: f?.terminal ?? null };

    const last = this.history[0] ?? '';
    this.input = InputState.new({ value: last, placeholder: 'Command to send…' });
    if (last) this.input.set_selected_range({ start: 0, end: last.length });
    this.input.on('change', (_e, cx) => cx.notify());
    this.input.on('submit', (_e, cx) => this.send(cx));
    this.input.focus();
    cx.timer.after(600, () => this.input.focus());
    this.handle = cx.focus_handle();
  }

  savePrefs() {
    setShared(PREFS, {
      scope: this.scope,
      machines: [...this.machines],
      skipAgents: this.skipAgents,
      includeSelf: this.includeSelf,
    });
  }

  // The terminals the text goes to, in space, tab, pane order.
  targets() {
    const tabs = new Map(); // tab id -> { space, tab }
    for (const s of spaces()) for (const t of s.tabs) tabs.set(t.id, { space: s, tab: t });
    const seen = new Set();
    const out = [];
    for (const p of panes()) {
      if (!p.terminal || seen.has(p.terminal)) continue;
      const where = tabs.get(p.tab);
      if (!where) continue;
      if (this.scope === 'tab' && p.tab !== this.origin.tab) continue;
      if (this.scope === 'space' && where.space.id !== this.origin.space) continue;
      if (this.scope === 'all' && !this.machines.has(machineKey(where.space.machine))) continue;
      const agent = p.status != null || p.program?.agent === true;
      if (this.skipAgents && agent) continue;
      if (!this.includeSelf && p.terminal === this.origin.terminal) continue;
      seen.add(p.terminal);
      out.push({ address: p.terminal, pane: p, space: where.space, tab: where.tab, agent });
    }
    return out;
  }

  send(cx) {
    if (this.sending) return;
    const text = this.input.value();
    if (!text.trim()) return;
    const targets = this.targets();
    if (targets.length === 0) {
      notify({ title: 'Broadcast', body: 'No terminals match.', level: 'warning' });
      return;
    }
    this.history = [text, ...this.history.filter((h) => h !== text)].slice(0, HISTORY_MAX);
    setShared(HISTORY, this.history);
    this.savePrefs();
    this.sending = true;
    cx.notify();
    cx.spawn(async (cx) => {
      const results = await Promise.all(
        targets.map((t) =>
          hc(['send', t.address, text])
            .then((r) => ({ t, ok: r.code === 0, why: (r.stderr || r.stdout).trim() }))
            .catch((e) => ({ t, ok: false, why: String(e) }))
        )
      );
      const failed = results.filter((r) => !r.ok);
      const sent = results.length - failed.length;
      if (failed.length === 0) {
        notify({
          title: 'Broadcast',
          body: `Sent to ${sent} terminal${sent === 1 ? '' : 's'}.`,
          level: 'success',
          timeout: 3000,
        });
      } else {
        const lines = failed.slice(0, 5).map((r) => `${r.t.address}: ${r.why || 'failed'}`);
        if (failed.length > 5) lines.push(`…and ${failed.length - 5} more`);
        notify({
          title: `Broadcast: ${failed.length} of ${results.length} failed`,
          body: lines.join('\n'),
          level: sent ? 'warning' : 'error',
        });
      }
      this.sending = false;
      closeOverlay(props().overlay);
      cx.notify();
    });
  }

  stepHistory(dir) {
    if (this.history.length === 0) return false;
    if (this.historyAt === -1) this.draft = this.input.value();
    const next = Math.max(-1, Math.min(this.history.length - 1, this.historyAt + dir));
    if (next === this.historyAt) return true;
    this.historyAt = next;
    const value = next === -1 ? this.draft : this.history[next];
    this.input.set_value(value);
    this.input.set_selected_range({ start: value.length, end: value.length });
    return true;
  }

  setScope(scope) {
    this.scope = scope;
    this.savePrefs();
  }
  setMachines(preset) {
    this.machines = machineSet(preset);
    this.savePrefs();
  }
  toggleMachine(k) {
    const next = new Set(this.machines);
    if (next.has(k)) next.delete(k);
    else next.add(k);
    this.machines = next;
    this.savePrefs();
  }

  key(e, cx) {
    const ks = e.keystroke;
    const scope = SCOPES.find((s) => s[2] === ks);
    let handled = true;
    if (ks === 'escape') closeOverlay(props().overlay);
    else if (scope) this.setScope(scope[0]);
    else if (ks === 'alt-l') {
      this.scope = 'all';
      this.setMachines('local');
    } else if (ks === 'alt-r') {
      this.scope = 'all';
      this.setMachines('remote');
    } else if (ks === 'alt-a') {
      this.scope = 'all';
      this.setMachines('all');
    } else if (ks === 'alt-g') {
      this.skipAgents = !this.skipAgents;
      this.savePrefs();
    } else if (ks === 'alt-i') {
      this.includeSelf = !this.includeSelf;
      this.savePrefs();
    } else if (ks === 'up') handled = this.stepHistory(1);
    else if (ks === 'down') handled = this.stepHistory(-1);
    else handled = false;
    if (handled) {
      cx.stop_propagation();
      cx.notify();
    }
  }

  render() {
    const t = theme();
    const targets = this.targets();
    const all = knownMachines();
    const sameSet = (keys) => keys.length === this.machines.size && keys.every((k) => this.machines.has(k));
    const preset = sameSet([LOCAL])
      ? 'local'
      : sameSet(all.filter((k) => k !== LOCAL))
        ? 'remote'
        : sameSet(all)
          ? 'all'
          : null;

    const label = (text, hint) =>
      h_flex()
        .pt(14)
        .pb(6)
        .items_center()
        .child(div().flex_1().text_size(11.5).text_color(t.dim).child(text))
        .when(Boolean(hint), (el) => el.child(div().text_size(10.5).text_color(t.dim).child(hint)));
    const chip = (id, text, on, action, disabled = false) =>
      h_flex()
        .id(id)
        .h(28)
        .px(11)
        .items_center()
        .rounded(7)
        .border_1()
        .border_color(on ? t.accent : t.input)
        .bg(on ? t.selected : t.raised)
        .text_size(12)
        .text_color(disabled ? t.dim : on ? t.foreground : t.muted)
        .when(!disabled, (el) => el.cursor_pointer().hover((s) => s.bg(t.hover)))
        .on_click((_e, cx) => {
          if (!disabled) {
            action();
            cx.notify();
          }
        })
        .child(text);
    const check = (id, text, on, action) =>
      h_flex()
        .id(id)
        .gap(7)
        .items_center()
        .cursor_pointer()
        .on_click((_e, cx) => {
          action();
          cx.notify();
        })
        .child(
          div()
            .w(14)
            .h(14)
            .rounded(4)
            .border_1()
            .border_color(on ? t.accent : t.input)
            .bg(on ? t.accent : t.background)
            .text_size(10)
            .text_color(t.accent_foreground)
            .items_center()
            .justify_center()
            .child(on ? '✓' : '')
        )
        .child(div().text_size(12).text_color(t.muted).child(text));
    const button = (id, text, primary, action) =>
      h_flex()
        .id(id)
        .h(32)
        .px(14)
        .items_center()
        .rounded(8)
        .border_1()
        .border_color(primary ? t.solid : t.input)
        .bg(primary ? t.solid : t.raised)
        .text_color(primary ? t.solid_foreground : t.foreground)
        .text_size(12.5)
        .font_medium()
        .cursor_pointer()
        .hover((s) => s.opacity(0.88))
        .on_click(action)
        .child(text);

    const machinesActive = this.scope === 'all';
    const row = (x) =>
      h_flex()
        .gap(8)
        .px(8)
        .py(3)
        .items_center()
        .text_size(11.5)
        .child(div().w(76).text_color(t.accent).truncate().child(x.address))
        .child(div().flex_1().truncate().text_color(t.muted).child(`${x.space.name} / ${x.tab.name}`))
        .child(
          div()
            .text_color(x.agent ? t.working : t.dim)
            .child(x.pane.program?.name ?? '')
        );

    return v_flex()
      .id('broadcast')
      .track_focus(this.handle)
      .on_key_down((e, cx) => this.key(e, cx))
      .size_full()
      .p(18)
      .bg(t.popover)
      .text_color(t.foreground)
      .child(
        h_flex()
          .items_center()
          .child(div().flex_1().text_size(15).font_semibold().child('Broadcast'))
          .child(div().text_size(11).text_color(t.dim).child('Enter sends · Esc · ↑↓ history'))
      )
      .child(label('Command'))
      .child(
        Input.new(this.input)
          .h(36)
          .px(10)
          .rounded(9)
          .border_1()
          .border_color(t.accent)
          .bg(t.background)
          .text_size(12.8)
          .text_color(t.foreground)
      )
      .child(label('Send to', 'alt-1 · alt-2 · alt-3'))
      .child(
        h_flex()
          .gap(6)
          .children(SCOPES.map(([id, text]) => chip(`scope-${id}`, text, this.scope === id, () => this.setScope(id))))
      )
      .child(label(machinesActive ? 'Machines' : 'Machines (for all spaces)', 'alt-l local · alt-r remote · alt-a all'))
      .child(
        h_flex()
          .gap(6)
          .flex_wrap()
          .gap_y(6)
          .child(
            chip(
              'm-local',
              'Local only',
              machinesActive && preset === 'local',
              () => this.setMachines('local'),
              !machinesActive
            )
          )
          .child(
            chip(
              'm-remote',
              'Remote only',
              machinesActive && preset === 'remote',
              () => this.setMachines('remote'),
              !machinesActive || all.length < 2
            )
          )
          .child(
            chip(
              'm-all',
              'All machines',
              machinesActive && preset === 'all',
              () => this.setMachines('all'),
              !machinesActive
            )
          )
          .child(div().w(1).h(20).mx(4).bg(t.border))
          .children(
            all.map((k) =>
              chip(
                `m-${k || 'here'}`,
                machineLabel(k),
                machinesActive && this.machines.has(k),
                () => this.toggleMachine(k),
                !machinesActive
              )
            )
          )
      )
      .child(
        h_flex()
          .pt(14)
          .gap(18)
          .child(
            check('skip-agents', 'Skip agents (alt-g)', this.skipAgents, () => {
              this.skipAgents = !this.skipAgents;
              this.savePrefs();
            })
          )
          .child(
            check('include-self', 'Include focused terminal (alt-i)', this.includeSelf, () => {
              this.includeSelf = !this.includeSelf;
              this.savePrefs();
            })
          )
      )
      .child(label(`${targets.length} terminal${targets.length === 1 ? '' : 's'}`))
      .child(
        v_flex()
          .id('targets')
          .flex_1()
          .min_h(0)
          .overflow_y_scrollbar()
          .rounded(8)
          .border_1()
          .border_color(t.border)
          .bg(t.background)
          .py(4)
          .when(targets.length === 0, (el) =>
            el.child(div().px(8).py(3).text_size(11.5).text_color(t.dim).child('No terminals match.'))
          )
          .children(targets.map(row))
      )
      .child(
        h_flex()
          .pt(14)
          .gap(8)
          .justify_end()
          .child(button('bc-cancel', 'Cancel', false, () => closeOverlay(props().overlay)))
          .child(
            button('bc-send', this.sending ? 'Sending…' : `Send to ${targets.length}`, true, (_e, cx) => this.send(cx))
          )
      );
  }
}
