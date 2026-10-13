// A short, soft notification sound made with WebAudio (no audio files).
// "done": rising two-note chirp. "waiting": two gentle falling notes.
let ctx: AudioContext | null = null

export function chirp(kind: 'done' | 'waiting') {
  try {
    ctx ??= new AudioContext()
    if (ctx.state === 'suspended') void ctx.resume()
    const notes = kind === 'done' ? [880, 1320] : [740, 554]
    notes.forEach((freq, i) => {
      const t = ctx!.currentTime + i * 0.11
      const osc = ctx!.createOscillator()
      const gain = ctx!.createGain()
      osc.type = 'sine'
      osc.frequency.setValueAtTime(freq, t)
      gain.gain.setValueAtTime(0.0001, t)
      gain.gain.exponentialRampToValueAtTime(0.12, t + 0.015)
      gain.gain.exponentialRampToValueAtTime(0.0001, t + 0.16)
      osc.connect(gain).connect(ctx!.destination)
      osc.start(t)
      osc.stop(t + 0.18)
    })
  } catch {
    /* audio unavailable: the visual notice still shows */
  }
}

const MUTE_KEY = 'maestro.chirpMuted'
export function loadMuted(): boolean {
  try { return localStorage.getItem(MUTE_KEY) === '1' } catch { return false }
}
export function saveMuted(m: boolean) {
  try { localStorage.setItem(MUTE_KEY, m ? '1' : '0') } catch { /* per-viewer convenience only */ }
}
