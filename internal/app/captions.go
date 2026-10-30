package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"netease-music-mygo/internal/lyrics"
	"netease-music-mygo/internal/speech"
)

// An episode's captions are written down on the device, as it plays:
// from where the listener is, ahead of them, and kept on disk, so that
// what was written down once is there the next time. A seek past what is
// written down starts over from there, leaving what was for later.

// These find, download and run the model; tests replace them.
var (
	captionsAvailable = speech.Available
	modelInstalled    = speech.Installed
	downloadModel     = speech.Download
	removeModel       = speech.Remove
	newTranscriber    = func(dir string) (transcriber, error) { return speech.NewRecognizer(dir) }
)

// A transcriber writes down what an episode says (speech.Recognizer).
type transcriber interface {
	Transcribe(ctx context.Context, src speech.Source, from, until time.Duration, said func(speech.Segment), heard func(time.Duration)) error
	Close()
}

// The model loaded, held while an episode is written down, and the lock
// that one transcription at a time holds it under: a new one waits for
// the one it replaces to stop.
var recognizer struct {
	sync.Mutex
	t transcriber
}

// captionLead is how far ahead of the listener the transcription may be
// for a seek to wait for it, rather than start another from there.
const captionLead = 30 * time.Second

// captionState is what the app has of captions: the model, and the
// episode playing's transcript.
type captionState struct {
	model modelState

	program int64       // the episode the transcript is of, 0 for none
	tr      *transcript // nil while it loads
	lines   []lyrics.Line
	work    *captionWork  // the transcription running, or nil
	failed  int64         // the episode the transcription failed for
	saved   time.Duration // how far the transcript on disk is written down
}

// modelState is the model's: whether it is installed, or downloading.
type modelState struct {
	checked, installed bool
	downloading        bool
	done, total        int64
	err                string
	cancel             context.CancelFunc
}

// A transcript is what is written down of an episode, and which stretches
// of it all is written down for.
type transcript struct {
	Model    string           `json:"model"`
	Segments []speech.Segment `json:"segments"`
	Spans    []span           `json:"spans"`
}

// A span is a stretch of an episode, from its first to its second.
type span [2]time.Duration

// captionWork is a transcription running from from up to until, 0 for the
// episode's end, written down as far as at.
type captionWork struct {
	program     int64
	from, until time.Duration
	at          time.Duration
	cancel      context.CancelFunc
}

// modelDir is where the model and the libraries that run it download to.
func modelDir() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "models"), nil
}

// transcriptDir is where the transcripts of episodes are kept.
func transcriptDir() (string, error) {
	dir, err := mygo.App.Path(mygo.PathCache)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "transcripts"), nil
}

func transcriptPath(program int64) (string, error) {
	dir, err := transcriptDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, strconv.FormatInt(program, 10)+".json"), nil
}

// captionsShown reports whether the app can make captions on this
// platform. The listener may still have to download the model.
func (a *app) captionsShown() bool { return captionsAvailable() }

// modelReady reports whether the model is installed, looking once.
func (a *app) modelReady() bool {
	m := &a.captions.model
	if !m.checked {
		m.checked = true
		if dir, err := modelDir(); err == nil {
			m.installed = modelInstalled(dir)
		}
	}
	return m.installed
}

// captionsOn reports whether episodes are written down as they play.
func (a *app) captionsOn() bool {
	return a.settings.Captions && a.captionsShown() && a.modelReady()
}

// startModelDownload downloads the model, its progress shown as it goes.
func (a *app) startModelDownload() {
	m := &a.captions.model
	if m.downloading || m.installed {
		return
	}
	dir, err := modelDir()
	if err != nil {
		m.err = "无法保存模型"
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.downloading, m.err, m.cancel = true, "", cancel
	m.done, m.total = 0, speech.ModelSize()
	go func() {
		// Shown in steps of a megabyte: the download reads far smaller
		// pieces.
		var shown int64
		err := downloadModel(ctx, dir, func(done, total int64) {
			if done-shown < 1<<20 && done < total {
				return
			}
			shown = done
			a.post(func() {
				m.done, m.total = done, total
				if a.settingsWin != nil {
					a.settingsWin.Invalidate()
				}
			})
		})
		a.post(func() {
			if a.settingsWin != nil {
				a.settingsWin.Invalidate()
			}
			m.downloading, m.cancel = false, nil
			switch {
			case err == nil:
				m.installed, m.checked = true, true
				a.settings.Captions = true
				a.settings.save()
				a.syncCaptions()
			case ctx.Err() != nil:
			default:
				m.err = "下载失败，请检查网络后重试"
			}
		})
	}()
}

// cancelModelDownload stops the download, keeping what came in for the
// next.
func (a *app) cancelModelDownload() {
	if m := &a.captions.model; m.cancel != nil {
		m.cancel()
	}
}

// deleteModel stops the transcription, lets the model go and removes it.
func (a *app) deleteModel() {
	m := &a.captions.model
	if !m.installed {
		return
	}
	a.stopCaptions()
	m.installed = false
	dir, err := modelDir()
	if err != nil {
		return
	}
	go func() {
		recognizer.Lock()
		if recognizer.t != nil {
			recognizer.t.Close()
			recognizer.t = nil
		}
		removeModel(dir)
		recognizer.Unlock()
	}()
}

// syncCaptions keeps the episode playing written down ahead of the
// listener: it loads its transcript, and starts a transcription from the
// first of what is not written down from where they are, unless one
// running gets there soon. It runs whenever the player changes.
func (a *app) syncCaptions() {
	cs := &a.captions
	var program int64
	if tr := a.pl.Current(); tr != nil {
		program = tr.ProgramID
	}
	if program != cs.program {
		a.pauseCaptions()
		a.saveTranscript()
		cs.program, cs.tr, cs.lines = program, nil, nil
		if program != 0 && a.captionsShown() {
			a.loadTranscript(program)
		}
	}
	if program == 0 || !a.captionsOn() || cs.failed == program {
		a.stopCaptions()
		return
	}
	if cs.tr == nil || !a.pl.Ready() {
		// The model stays for once the episode or its transcript loads.
		a.pauseCaptions()
		return
	}
	from, until, ok := captionStart(cs.tr.Spans, a.pl.Position(), a.pl.Duration())
	if w := cs.work; w != nil {
		if ok && w.from <= from && from <= w.at+captionLead {
			return
		}
		w.cancel()
		cs.work = nil
	}
	if !ok {
		releaseRecognizer()
		return
	}
	a.transcribe(program, from, until)
}

// stopCaptions stops the transcription, if one runs, and lets the model
// go once it has.
func (a *app) stopCaptions() {
	a.pauseCaptions()
	releaseRecognizer()
}

// pauseCaptions stops the transcription, if one runs, keeping the model.
func (a *app) pauseCaptions() {
	if w := a.captions.work; w != nil {
		w.cancel()
		a.captions.work = nil
	}
}

// releaseRecognizer lets the model go, once no transcription holds it.
func releaseRecognizer() {
	go func() {
		recognizer.Lock()
		defer recognizer.Unlock()
		if recognizer.t != nil {
			recognizer.t.Close()
			recognizer.t = nil
		}
	}()
}

// transcribe writes down the episode playing from from up to until, on a
// goroutine.
func (a *app) transcribe(program int64, from, until time.Duration) {
	src, err := a.pl.OpenAudio()
	if err != nil {
		return
	}
	if err := src.SetPosition(int64(from.Seconds() * float64(src.SampleRate()))); err != nil {
		src.Close()
		return
	}
	dir, err := modelDir()
	if err != nil {
		src.Close()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	w := &captionWork{program: program, from: from, until: until, at: from, cancel: cancel}
	a.captions.work = w
	go func() {
		defer src.Close()
		recognizer.Lock()
		defer recognizer.Unlock()
		if ctx.Err() != nil {
			return
		}
		var err error
		if recognizer.t == nil {
			recognizer.t, err = newTranscriber(dir)
		}
		if err == nil {
			err = recognizer.t.Transcribe(ctx, src, from, until,
				func(s speech.Segment) { a.post(func() { a.captionSaid(w, s) }) },
				func(at time.Duration) { a.post(func() { a.captionHeard(w, at) }) })
		}
		a.post(func() { a.captionDone(w, err, ctx.Err() != nil) })
	}()
}

// captionSaid adds a segment written down to the transcript of its
// episode, unless the transcript has it: a transcription started again
// from where one stopped writes down again what that one did last.
func (a *app) captionSaid(w *captionWork, s speech.Segment) {
	cs := &a.captions
	if w.program != cs.program || cs.tr == nil {
		return
	}
	cs.tr.Segments = addSegment(cs.tr.Segments, s)
	cs.lines = speech.Lines(cs.tr.Segments)
}

// captionHeard marks the episode written down from where w started to at.
func (a *app) captionHeard(w *captionWork, at time.Duration) {
	cs := &a.captions
	if w.program != cs.program || cs.tr == nil {
		return
	}
	w.at = max(w.at, at)
	cs.tr.Spans = addSpan(cs.tr.Spans, span{w.from, at})
	// Kept a minute at a time, should the app not quit as it should.
	if at > cs.saved+time.Minute || at < cs.saved {
		cs.saved = at
		a.saveTranscript()
	}
}

// captionDone ends w: all it was to write down is, unless it was stopped
// or failed. Then the next of what is not written down follows.
func (a *app) captionDone(w *captionWork, err error, stopped bool) {
	cs := &a.captions
	if cs.work == w {
		cs.work = nil
	}
	if w.program != cs.program || cs.tr == nil || stopped {
		return
	}
	if err != nil {
		cs.failed = w.program
		a.saveTranscript()
		return
	}
	to := w.until
	if to == 0 {
		to = max(w.at, a.pl.Duration())
	}
	cs.tr.Spans = addSpan(cs.tr.Spans, span{w.from, to})
	a.saveTranscript()
	a.syncCaptions()
}

// captionsWritten reports whether all of the episode playing is written
// down.
func (a *app) captionsWritten() bool {
	cs := &a.captions
	if cs.tr == nil {
		return false
	}
	_, _, ok := captionStart(cs.tr.Spans, 0, a.pl.Duration())
	return !ok
}

// captionsWriting reports whether what is at pos is being written down
// and not yet there.
func (a *app) captionsWriting(pos time.Duration) bool {
	cs := &a.captions
	if cs.tr == nil {
		return a.captionsOn() && cs.program != 0
	}
	if _, ok := covered(cs.tr.Spans, pos); ok {
		return false
	}
	return cs.work != nil
}

// loadTranscript reads the transcript of an episode on a goroutine, or
// starts one with nothing in it.
func (a *app) loadTranscript(program int64) {
	path, err := transcriptPath(program)
	if err != nil {
		a.captions.tr = &transcript{Model: speech.ModelName}
		return
	}
	go func() {
		tr := &transcript{}
		if b, err := os.ReadFile(path); err != nil || json.Unmarshal(b, tr) != nil || tr.Model != speech.ModelName {
			tr = &transcript{Model: speech.ModelName}
		}
		a.post(func() {
			cs := &a.captions
			if cs.program == program && cs.tr == nil {
				cs.tr = tr
				cs.lines = speech.Lines(tr.Segments)
				a.syncCaptions()
			}
		})
	}()
}

// saveTranscript keeps the transcript of the episode on disk, on a
// goroutine.
func (a *app) saveTranscript() {
	if write := a.transcriptWrite(); write != nil {
		go write()
	}
}

// transcriptWrite returns what writes the transcript of the episode as
// it is now, or nil with nothing to write.
func (a *app) transcriptWrite() func() {
	cs := &a.captions
	if cs.program == 0 || cs.tr == nil || len(cs.tr.Spans) == 0 {
		return nil
	}
	path, err := transcriptPath(cs.program)
	if err != nil {
		return nil
	}
	b, err := json.Marshal(cs.tr)
	if err != nil {
		return nil
	}
	return func() {
		if os.MkdirAll(filepath.Dir(path), 0o755) == nil {
			tmp := path + ".tmp"
			if os.WriteFile(tmp, b, 0o644) == nil {
				os.Rename(tmp, path)
			}
		}
	}
}

// captionStart returns where to write down from for a listener at pos of
// an episode length long, of which spans are written down: pos, or the
// end of the span it is in, or once all from there on is written down,
// the first of what is not before it; and up to where: the start of the
// next span, or 0 for the end. It reports false once all of it is
// written down.
func captionStart(spans []span, pos, length time.Duration) (from, until time.Duration, ok bool) {
	if from, until, ok = captionAfter(spans, pos, length); ok || pos == 0 {
		return from, until, ok
	}
	return captionAfter(spans, 0, length)
}

// captionAfter is captionStart for what is from pos on alone.
func captionAfter(spans []span, pos, length time.Duration) (from, until time.Duration, ok bool) {
	from = pos
	if end, in := covered(spans, pos); in {
		from = end
	}
	if length > 0 && from >= length-time.Second {
		return 0, 0, false
	}
	for _, s := range spans {
		if s[0] > from {
			return from, s[0], true
		}
	}
	return from, 0, true
}

// covered returns the end of the span holding t, if one does.
func covered(spans []span, t time.Duration) (time.Duration, bool) {
	for _, s := range spans {
		if s[0] <= t && t < s[1] {
			return s[1], true
		}
	}
	return 0, false
}

// addSpan adds s to spans, sorted, merging those it meets.
func addSpan(spans []span, s span) []span {
	if s[1] <= s[0] {
		return spans
	}
	out := make([]span, 0, len(spans)+1)
	for _, o := range spans {
		if o[1] < s[0] || o[0] > s[1] {
			out = append(out, o)
			continue
		}
		s = span{min(s[0], o[0]), max(s[1], o[1])}
	}
	out = append(out, s)
	slices.SortFunc(out, func(x, y span) int { return int(x[0] - y[0]) })
	return out
}

// addSegment adds s to segs, sorted, unless it is mostly over one there.
func addSegment(segs []speech.Segment, s speech.Segment) []speech.Segment {
	i, _ := slices.BinarySearchFunc(segs, s.Start, func(x speech.Segment, t time.Duration) int { return int(x.Start - t) })
	for _, o := range segs[max(i-1, 0):min(i+1, len(segs))] {
		if overlap := min(o.End, s.End) - max(o.Start, s.Start); overlap > (s.End-s.Start)/2 {
			return segs
		}
	}
	return slices.Insert(segs, i, s)
}
