from __future__ import annotations

import copy
import json
import re
import sys
import unicodedata
from concurrent.futures import ThreadPoolExecutor, as_completed
from datetime import datetime, timedelta
from email import policy
from email.parser import BytesParser
from pathlib import Path
from typing import Any
from urllib.parse import parse_qs, quote_plus, urlparse
from urllib.request import Request, urlopen

from bs4 import BeautifulSoup, NavigableString, Tag


ROOT = Path(__file__).resolve().parent
NOTES_PATH = ROOT / "study_notes.json"
WOL_QUERY_BASE = "https://wol.jw.org/es/wol/l/r4/lp-s?q="
OUTPUT_HTML = ROOT / "atalaya-rescate-estudio.html"
OUTPUT_FILES = {
    "source": ROOT / "00-articulo-fuente.md",
    "guide": ROOT / "01-guia-conduccion.md",
    "scriptures": ROOT / "02-textos-biblicos.md",
    "terms": ROOT / "03-terminos-clave.md",
    "applications": ROOT / "04-aplicaciones-y-preguntas.md",
    "threads": ROOT / "05-relaciones-transversales.md",
    "timing": ROOT / "06-tiempos-conduccion.md",
}
BIBLE_TOOLTIP_CACHE = ROOT / ".bible_tooltips_cache.json"
ENABLE_REMOTE_BIBLE_TOOLTIPS = False

BIBLE_BOOK_ALIASES = {
    "rom": "romanos",
    "romanos": "romanos",
    "col": "colosenses",
    "colosenses": "colosenses",
    "ecl": "eclesiastes",
    "eclesiastes": "eclesiastes",
    "lev": "levitico",
    "levitico": "levitico",
    "num": "numeros",
    "numeros": "numeros",
    "sal": "salmos",
    "salmo": "salmos",
    "salmos": "salmos",
    "sant": "santiago",
    "santiago": "santiago",
    "efes": "efesios",
    "efesios": "efesios",
    "is": "isaias",
    "isaias": "isaias",
    "gen": "genesis",
    "genesis": "genesis",
    "mat": "mateo",
    "mateo": "mateo",
    "hech": "hechos",
    "hechos": "hechos",
    "juan": "juan",
    "prov": "proverbios",
    "proverbios": "proverbios",
    "1juan": "1juan",
    "1 juan": "1juan",
    "1cor": "1corintios",
    "1 cor": "1corintios",
    "1corintios": "1corintios",
}

LOCAL_CSS = """
:root {
  --study-accent: #6f1d1b;
  --study-ink: #1f2430;
  --study-muted: #5d6673;
  --study-border: #dbcab8;
  --study-card: rgba(255, 253, 249, 0.96);
  --study-link: #2d6fba;
  --study-shadow: 0 14px 28px rgba(51, 42, 35, 0.08);
  --study-purple: rgba(202, 176, 238, 0.7);
  --study-blue: rgba(178, 225, 255, 0.7);
  --study-green: rgba(210, 237, 174, 0.86);
  --study-pink: rgba(252, 195, 220, 0.76);
}

html {
  scroll-behavior: smooth;
}

body.study-page {
  margin: 0;
  background:
    radial-gradient(circle at top right, rgba(150, 176, 213, 0.12), transparent 20rem),
    linear-gradient(180deg, #fbf8f4, #f5f2ed 22rem, #fbfaf8);
  color: var(--study-ink);
}

.study-shell {
  max-width: 1700px;
  margin: 0 auto;
  padding: 0.7rem 1rem 4rem;
}

.study-layout {
  margin-top: 0;
}

.study-article-wrap {
  background: rgba(255, 255, 255, 0.94);
  border: 1px solid rgba(111, 29, 27, 0.1);
  border-radius: 18px;
  padding: 1.1rem 1.1rem 2rem;
  box-shadow: var(--study-shadow);
}

.study-article {
  width: 100%;
}

.study-article #article,
.study-article #article .scalableui,
.study-article #article .bodyTxt {
  width: 100% !important;
  max-width: none !important;
  margin-inline: 0 !important;
}

.study-article #article header,
.study-article #article header > *,
.study-article #article .contextTtl,
.study-article #article .scalableui > div,
.study-article #article img {
  max-width: 100% !important;
  box-sizing: border-box;
}

.study-article-wrap {
  overflow: hidden;
}

.study-article a,
.study-note-links a,
.study-ref-list a,
.study-panel a {
  color: var(--study-link);
  text-decoration: none;
}

.study-article a:hover,
.study-note-links a:hover,
.study-ref-list a:hover,
.study-panel a:hover {
  text-decoration: underline;
}

.study-hover-tooltip {
  position: fixed;
  z-index: 90;
  width: min(34rem, calc(100vw - 1.6rem));
  background: rgba(255, 255, 255, 0.985);
  border: 1px solid rgba(111, 29, 27, 0.16);
  border-radius: 16px;
  box-shadow: 0 22px 46px rgba(36, 35, 31, 0.22);
  overflow: hidden;
}

.study-hover-tooltip[hidden] {
  display: none;
}

.study-hover-tooltip-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  padding: 0.9rem 1rem 0.75rem;
  background: #ece8e2;
  border-bottom: 1px solid rgba(111, 29, 27, 0.1);
}

.study-hover-tooltip-kicker {
  margin: 0;
  font-size: 0.84rem;
  color: var(--study-muted);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.study-hover-tooltip-title {
  margin: 0.12rem 0 0;
  font-size: 1.24rem;
  line-height: 1.2;
}

.study-hover-tooltip-title a {
  color: var(--study-link);
}

.study-hover-tooltip-subtitle {
  margin: 0.24rem 0 0;
  font-size: 0.94rem;
  color: var(--study-muted);
}

.study-hover-tooltip-close {
  border: 0;
  background: transparent;
  color: var(--study-muted);
  font-size: 1.7rem;
  line-height: 1;
  cursor: pointer;
  padding: 0 0.1rem;
}

.study-hover-tooltip-body {
  max-height: min(26rem, 62vh);
  overflow: auto;
  padding: 1rem 1.1rem 1.1rem;
  scrollbar-width: thin;
}

.study-hover-tooltip-body::-webkit-scrollbar {
  width: 8px;
}

.study-hover-tooltip-body::-webkit-scrollbar-thumb {
  background: rgba(111, 29, 27, 0.16);
  border-radius: 999px;
}

.study-hover-tooltip-verse {
  margin: 0;
  font-size: 1.07rem;
  line-height: 1.56;
}

.study-hover-tooltip-verse + .study-hover-tooltip-verse {
  margin-top: 0.45rem;
}

.study-hover-tooltip-verse-num {
  color: var(--study-link);
  font-weight: 800;
  margin-inline-end: 0.42rem;
}

.study-article .study-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(230px, 19rem);
  gap: 0.36rem;
  align-items: start;
  margin: 0.52rem 0 0.82rem;
  padding-inline-start: 0.5rem;
  border-inline-start: 4px solid rgba(111, 29, 27, 0.12);
}

.study-row.introductorio {
  border-inline-start-color: rgba(120, 162, 219, 0.7);
}

.study-row.troncal {
  border-inline-start-color: rgba(116, 161, 74, 0.72);
}

.study-row.cierre {
  border-inline-start-color: rgba(168, 90, 143, 0.75);
}

.study-row.is-current {
  background: rgba(255, 248, 232, 0.68);
  border-radius: 14px;
  padding: 0.75rem 0.9rem 1rem 1rem;
  box-shadow: 0 0 0 1px rgba(162, 114, 29, 0.18);
}

.study-row.is-current .study-para {
  color: #111;
}

.study-row.is-past {
  opacity: 0.93;
}

.study-article .study-para {
  margin: 0;
  font-size: clamp(1.18rem, 1.06rem + 0.72vw, 1.62rem);
  line-height: 1.52;
  max-width: none;
  grid-column: 1;
}

.study-article .study-para .parNum[data-note-target] {
  cursor: pointer;
}

.study-article .study-para .parNum[data-note-target]:hover,
.study-article .study-para .parNum[data-note-target]:focus-visible {
  color: #215d9d;
}

.study-article .study-para .parNum[data-note-target]:focus-visible {
  outline: 2px solid rgba(77, 143, 229, 0.38);
  outline-offset: 3px;
  border-radius: 8px;
}

.study-question {
  width: 100%;
  box-sizing: border-box;
  background: rgba(111, 29, 27, 0.04);
  border: 1px solid rgba(111, 29, 27, 0.12);
  border-radius: 12px;
  padding: 0.8rem 0.95rem;
  margin: 0.8rem 0 0.55rem;
  font-size: 1rem;
}

.study-mark {
  padding: 0 0.05rem;
  border-radius: 0.12rem;
  box-decoration-break: clone;
  -webkit-box-decoration-break: clone;
}

.study-mark-primary {
  background: var(--study-purple);
}

.study-mark-secondary {
  background: var(--study-blue);
}

.study-mark-key {
  background: var(--study-green);
}

.study-mark-support {
  background: var(--study-pink);
}

.study-note-card {
  grid-column: 2;
  display: flex;
  flex-direction: column;
  background: rgba(255, 253, 250, 0.98);
  border: 1px solid rgba(111, 29, 27, 0.14);
  border-radius: 16px;
  box-shadow: 0 10px 22px rgba(51, 42, 35, 0.06);
  padding: 0.58rem 0.62rem 0.64rem;
  min-width: 0;
  width: 100%;
  max-width: 100%;
  max-height: clamp(11rem, 26vh, 15.5rem);
  overflow: hidden;
  box-sizing: border-box;
}

.study-row.is-current .study-note-card {
  border-color: rgba(162, 114, 29, 0.42);
  box-shadow: 0 14px 28px rgba(129, 94, 28, 0.12);
}

.study-note-card.is-target {
  border-color: rgba(77, 143, 229, 0.52);
  box-shadow: 0 0 0 2px rgba(77, 143, 229, 0.16), 0 16px 30px rgba(52, 85, 128, 0.14);
}

.study-row:hover .study-note-card {
  border-color: rgba(77, 143, 229, 0.36);
  box-shadow: 0 14px 30px rgba(52, 85, 128, 0.12);
}

.study-note-head {
  display: flex;
  align-items: flex-start;
  gap: 0.58rem;
  margin-bottom: 0.32rem;
  flex: 0 0 auto;
}

.study-note-number {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 1.8rem;
  height: 1.8rem;
  border-radius: 0.68rem;
  background: rgba(77, 143, 229, 0.12);
  color: #215d9d;
  font-weight: 800;
  font-size: 0.96rem;
  flex: 0 0 auto;
}

.study-note-meta {
  min-width: 0;
}

.study-note-body {
  min-height: 0;
  overflow-y: auto;
  overscroll-behavior: contain;
  padding-inline-end: 0.14rem;
  scrollbar-width: thin;
}

.study-note-body::-webkit-scrollbar {
  width: 7px;
}

.study-note-body::-webkit-scrollbar-thumb {
  background: rgba(111, 29, 27, 0.16);
  border-radius: 999px;
}

.study-note-body h4,
.study-note-section h4 {
  margin: 0 0 0.28rem;
  color: var(--study-accent);
  font-size: 0.84rem;
}

.study-note-body p,
.study-note-section p {
  margin: 0.16rem 0 0;
  font-size: 0.8rem;
  line-height: 1.32;
}

.study-note-section + .study-note-section {
  margin-top: 0.5rem;
  padding-top: 0.48rem;
  border-top: 1px solid rgba(111, 29, 27, 0.1);
}

.study-note-section.concept {
  background: rgba(214, 237, 193, 0.36);
  border: 1px solid rgba(128, 175, 91, 0.28);
  border-radius: 12px;
  padding: 0.58rem 0.64rem;
}

.study-note-section.scripture {
  background: rgba(228, 240, 253, 0.38);
  border-radius: 12px;
  padding: 0.58rem 0.64rem;
}

.study-note-section.thread {
  background: rgba(249, 237, 246, 0.42);
  border-radius: 12px;
  padding: 0.58rem 0.64rem;
}

.study-note-section.application {
  background: rgba(255, 247, 233, 0.55);
  border-radius: 12px;
  padding: 0.58rem 0.64rem;
}

.study-note .study-badges,
.study-note-card .study-badges {
  display: flex;
  flex-wrap: wrap;
  gap: 0.34rem;
  margin: 0;
}

.study-badge {
  display: inline-flex;
  align-items: center;
  gap: 0.28rem;
  padding: 0.2rem 0.56rem;
  border-radius: 999px;
  font-size: 0.78rem;
  font-weight: 700;
  background: rgba(111, 29, 27, 0.09);
  color: var(--study-accent);
}

.study-badge.time {
  background: rgba(45, 111, 186, 0.12);
  color: #155893;
}

.study-badge.note {
  background: rgba(162, 114, 29, 0.12);
  color: #865b12;
}

.study-note-citations {
  display: flex;
  flex-wrap: wrap;
  gap: 0.42rem;
  margin-top: 0.45rem;
}

.study-note-citations a {
  display: inline-flex;
  align-items: center;
  padding: 0.18rem 0.5rem;
  border-radius: 999px;
  background: rgba(45, 111, 186, 0.08);
  color: var(--study-link);
  font-size: 0.76rem;
  text-decoration: none;
}

.study-note-miniquestion {
  font-weight: 700;
  color: #233247;
}

.study-article .groupFootnote,
.study-article #tt37,
.study-article .blockTeach {
  background: rgba(111, 29, 27, 0.04);
  border: 1px solid rgba(111, 29, 27, 0.11);
  border-radius: 14px;
  padding: 0.95rem 1rem;
  margin-top: 1.2rem;
}

.study-article .blockTeach ul {
  margin-bottom: 0;
}

.study-dock {
  position: sticky;
  top: 3.1rem;
  z-index: 29;
  margin: 0 0 0.75rem auto;
  width: min(310px, 100%);
  background: rgba(255, 253, 250, 0.96);
  border: 1px solid rgba(111, 29, 27, 0.16);
  border-radius: 14px;
  box-shadow: 0 12px 24px rgba(34, 28, 24, 0.12);
  backdrop-filter: blur(10px);
  padding: 0.52rem 0.64rem 0.58rem;
}

.study-top-actions {
  position: sticky;
  top: 0.55rem;
  z-index: 31;
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
  margin-bottom: 0.35rem;
}

.study-top-actions button {
  border: 0;
  border-radius: 999px;
  background: rgba(255, 253, 250, 0.96);
  color: var(--study-accent);
  padding: 0.44rem 0.76rem;
  font: inherit;
  font-weight: 700;
  box-shadow: 0 10px 18px rgba(34, 28, 24, 0.1);
  cursor: pointer;
}

.study-dock-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.8rem;
  margin-bottom: 0.34rem;
}

.study-dock-title {
  font-size: 0.84rem;
  font-weight: 700;
  color: var(--study-accent);
}

.study-dock-status {
  font-size: 0.8rem;
  color: var(--study-ink);
}

.study-dock-meta {
  margin-top: 0.08rem;
  color: var(--study-muted);
  font-size: 0.74rem;
}

.study-dock button,
.study-panel button {
  border: 0;
  border-radius: 999px;
  background: var(--study-accent);
  color: #fff;
  padding: 0.44rem 0.72rem;
  font: inherit;
  font-weight: 700;
  cursor: pointer;
}

.study-dock button.secondary,
.study-panel button.secondary {
  background: rgba(111, 29, 27, 0.1);
  color: var(--study-accent);
}

.study-progress {
  position: relative;
  margin-top: 0.36rem;
}

.study-progress-track {
  position: relative;
  display: flex;
  height: 10px;
  overflow: hidden;
  border-radius: 999px;
  background: rgba(111, 29, 27, 0.08);
}

.study-progress-segment {
  position: relative;
  border-inline-end: 1px solid rgba(255, 255, 255, 0.5);
  background: rgba(111, 29, 27, 0.18);
}

.study-progress-segment.introductorio {
  background: rgba(120, 162, 219, 0.42);
}

.study-progress-segment.troncal {
  background: rgba(128, 175, 91, 0.48);
}

.study-progress-segment.cierre {
  background: rgba(184, 105, 155, 0.42);
}

.study-progress-segment.is-current {
  box-shadow: inset 0 0 0 2px rgba(111, 29, 27, 0.5);
}

.study-progress-marker {
  position: absolute;
  top: -3px;
  bottom: -3px;
  width: 3px;
  background: #111;
  border-radius: 999px;
  transform: translateX(-50%);
}

.study-progress-scale {
  display: flex;
  justify-content: space-between;
  margin-top: 0.24rem;
  font-size: 0.68rem;
  color: var(--study-muted);
}

.study-progress-minutes {
  display: flex;
  justify-content: space-between;
  margin-top: 0.2rem;
  font-size: 0.62rem;
  color: var(--study-muted);
}

.study-progress-minutes span {
  min-width: 2rem;
  text-align: center;
}

.study-panel-backdrop {
  position: fixed;
  inset: 0;
  z-index: 40;
  background: rgba(18, 20, 24, 0.28);
}

.study-panel {
  position: fixed;
  top: 0;
  right: 0;
  bottom: 0;
  z-index: 41;
  width: min(460px, 100vw);
  background: rgba(255, 252, 248, 0.98);
  border-inline-start: 1px solid rgba(111, 29, 27, 0.14);
  box-shadow: -18px 0 36px rgba(34, 28, 24, 0.12);
  padding: 1rem 1rem 1.3rem;
  overflow-y: auto;
}

.study-panel[hidden],
.study-panel-backdrop[hidden] {
  display: none;
}

.study-panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.8rem;
  margin-bottom: 0.8rem;
}

.study-panel h2,
.study-panel h3 {
  margin: 0;
  color: var(--study-accent);
}

.study-panel-section {
  margin-top: 1rem;
  background: var(--study-card);
  border: 1px solid rgba(111, 29, 27, 0.1);
  border-radius: 14px;
  padding: 0.9rem;
}

.study-controls {
  display: grid;
  grid-template-columns: 1fr;
  gap: 0.75rem;
}

.study-controls label {
  display: block;
  font-size: 0.9rem;
  font-weight: 700;
  color: var(--study-muted);
  margin-bottom: 0.3rem;
}

.study-controls input,
.study-controls output {
  width: 100%;
  box-sizing: border-box;
  padding: 0.72rem 0.8rem;
  border: 1px solid var(--study-border);
  border-radius: 10px;
  background: #fff;
  font: inherit;
}

.study-controls output {
  display: inline-flex;
  align-items: center;
  min-height: 2.8rem;
}

.study-current-card {
  font-size: 0.95rem;
  line-height: 1.45;
}

.study-current-card strong {
  color: var(--study-accent);
}

.study-timing-table {
  width: 100%;
  border-collapse: collapse;
  margin-top: 0.5rem;
  font-size: 0.92rem;
}

.study-timing-table th,
.study-timing-table td {
  border-bottom: 1px solid rgba(111, 29, 27, 0.12);
  padding: 0.46rem 0.3rem;
  text-align: left;
  vertical-align: top;
}

.study-timing-table tr.is-current-row td {
  background: rgba(210, 237, 174, 0.26);
}

.study-legend {
  display: grid;
  grid-template-columns: 1fr;
  gap: 0.45rem;
  margin-top: 0.65rem;
}

.study-legend-item {
  display: flex;
  align-items: center;
  gap: 0.55rem;
  font-size: 0.9rem;
}

.study-legend-swatch {
  width: 18px;
  height: 18px;
  border-radius: 6px;
  box-shadow: inset 0 0 0 1px rgba(0, 0, 0, 0.06);
}

@media (max-width: 1220px) {
  .study-article .study-row {
    grid-template-columns: minmax(0, 1fr);
    gap: 0.7rem;
  }

  .study-note-card {
    grid-column: 1;
    max-height: none;
  }
}

@media (max-width: 760px) {
  .study-shell {
    padding: 0.55rem 0.45rem 4.2rem;
  }

  .study-article-wrap {
    padding: 0.8rem 0.7rem 1.6rem;
    border-radius: 14px;
  }

  .study-article .study-para {
    font-size: 1.06rem;
  }

  .study-row.is-current {
    padding-inline: 0.7rem;
  }

  .study-top-actions {
    top: 0.45rem;
  }

  .study-dock {
    top: 2.8rem;
    width: auto;
    margin-inline: 0;
  }
}
"""

LOCAL_JS = """
const timingConfig = JSON.parse(document.getElementById('study-timing-data').textContent);
const bibleTooltipData = JSON.parse(document.getElementById('study-bible-tooltip-data')?.textContent || '{}');
const bibleTooltip = document.getElementById('study-hover-tooltip');
const bibleTooltipTitle = document.getElementById('study-hover-tooltip-title');
const bibleTooltipSubtitle = document.getElementById('study-hover-tooltip-subtitle');
const bibleTooltipBody = document.getElementById('study-hover-tooltip-body');
const bibleTooltipClose = document.getElementById('study-hover-tooltip-close');
let bibleTooltipShowTimer = null;
let bibleTooltipHideTimer = null;
let currentTooltipTrigger = null;

function formatTime(date) {
  return date.toLocaleTimeString('es-BO', {
    hour: 'numeric',
    minute: '2-digit',
    hour12: true
  }).replace(' p. m.', ' p. m.').replace(' a. m.', ' a. m.');
}

function clamp(value, min, max) {
  return Math.min(Math.max(value, min), max);
}

function clearBibleTooltipTimers() {
  window.clearTimeout(bibleTooltipShowTimer);
  window.clearTimeout(bibleTooltipHideTimer);
}

function hideBibleTooltip() {
  clearBibleTooltipTimers();
  if (!bibleTooltip) {
    return;
  }
  bibleTooltip.hidden = true;
  currentTooltipTrigger = null;
}

function positionBibleTooltip(trigger) {
  if (!bibleTooltip || !trigger) {
    return;
  }
  const rect = trigger.getBoundingClientRect();
  const margin = 12;
  const width = bibleTooltip.offsetWidth;
  const height = bibleTooltip.offsetHeight;
  const left = clamp(rect.left + (rect.width / 2) - (width / 2), margin, window.innerWidth - width - margin);
  let top = rect.bottom + 12;
  if (top + height > window.innerHeight - margin) {
    top = rect.top - height - 12;
  }
  if (top < margin) {
    top = margin;
  }
  bibleTooltip.style.left = `${left}px`;
  bibleTooltip.style.top = `${top}px`;
}

function renderBibleTooltip(trigger) {
  if (!bibleTooltip) {
    return;
  }
  const key = trigger.dataset.studyTooltipKey;
  const payload = bibleTooltipData[key];
  if (!payload) {
    return;
  }

  bibleTooltipTitle.textContent = payload.title || trigger.textContent.trim();
  bibleTooltipTitle.href = payload.href || trigger.href || '#';
  bibleTooltipSubtitle.textContent = payload.subtitle || '';
  bibleTooltipBody.innerHTML = '';

  (payload.verses || []).forEach((verse) => {
    const paragraph = document.createElement('p');
    paragraph.className = 'study-hover-tooltip-verse';
    const number = document.createElement('span');
    number.className = 'study-hover-tooltip-verse-num';
    number.textContent = `${verse.number}`;
    paragraph.appendChild(number);
    paragraph.append(document.createTextNode(verse.text));
    bibleTooltipBody.appendChild(paragraph);
  });

  bibleTooltip.hidden = false;
  currentTooltipTrigger = trigger;
  positionBibleTooltip(trigger);
}

function queueBibleTooltip(trigger) {
  clearBibleTooltipTimers();
  bibleTooltipShowTimer = window.setTimeout(() => {
    renderBibleTooltip(trigger);
  }, 720);
}

function queueBibleTooltipHide() {
  clearBibleTooltipTimers();
  bibleTooltipHideTimer = window.setTimeout(() => {
    hideBibleTooltip();
  }, 180);
}

function bindBibleTooltips() {
  if (!bibleTooltip) {
    return;
  }

  document.querySelectorAll('[data-study-tooltip-key]').forEach((trigger) => {
    trigger.addEventListener('mouseenter', () => queueBibleTooltip(trigger));
    trigger.addEventListener('mouseleave', () => queueBibleTooltipHide());
    trigger.addEventListener('focus', () => renderBibleTooltip(trigger));
    trigger.addEventListener('blur', () => queueBibleTooltipHide());
  });

  bibleTooltip.addEventListener('mouseenter', () => {
    clearBibleTooltipTimers();
  });
  bibleTooltip.addEventListener('mouseleave', () => {
    queueBibleTooltipHide();
  });

  bibleTooltipClose?.addEventListener('click', hideBibleTooltip);
  document.addEventListener('keydown', (event) => {
    if (event.key === 'Escape') {
      hideBibleTooltip();
    }
  });
  window.addEventListener('resize', () => {
    if (!bibleTooltip.hidden && currentTooltipTrigger) {
      positionBibleTooltip(currentTooltipTrigger);
    }
  });
  document.addEventListener('scroll', () => {
    if (!bibleTooltip.hidden && currentTooltipTrigger) {
      positionBibleTooltip(currentTooltipTrigger);
    }
  }, true);
}

function buildSchedule(start) {
  const introEnd = new Date(start.getTime() + timingConfig.intro_minutes * 60000);
  let cursor = introEnd;
  const paragraphs = timingConfig.paragraphs.map((row) => {
    const rowStart = new Date(cursor);
    const rowEnd = new Date(cursor.getTime() + row.timing_minutes * 60000);
    cursor = rowEnd;
    return { ...row, start: rowStart, end: rowEnd };
  });

  const closingStart = new Date(cursor);
  const closingEnd = new Date(cursor.getTime() + timingConfig.closing_minutes * 60000);
  const blocks = timingConfig.blocks.map((block) => {
    const blockStart = new Date(start.getTime() + block.offset_minutes * 60000);
    const blockEnd = new Date(blockStart.getTime() + block.minutes * 60000);
    return { ...block, start: blockStart, end: blockEnd };
  });

  return { introEnd, paragraphs, blocks, closingStart, closingEnd };
}

function setCurrentParagraphState(paragraphNumber) {
  document.querySelectorAll('.study-row').forEach((row) => {
    const value = Number(row.querySelector('.study-para')?.dataset.studyNumber || 0);
    row.classList.toggle('is-current', value === paragraphNumber);
    row.classList.toggle('is-past', paragraphNumber > 0 && value < paragraphNumber);
  });

  document.querySelectorAll('.study-progress-segment').forEach((segment) => {
    const value = Number(segment.dataset.studyNumber || 0);
    segment.classList.toggle('is-current', value === paragraphNumber);
  });

  document.querySelectorAll('.study-timing-table tbody tr[data-study-row]').forEach((row) => {
    const value = Number(row.dataset.studyRow || 0);
    row.classList.toggle('is-current-row', value === paragraphNumber);
  });
}

function setCurrentBlockState(blockLabel) {
  document.querySelectorAll('.study-timing-table tbody tr[data-block-row]').forEach((row) => {
    row.classList.toggle('is-current-row', row.dataset.blockRow === blockLabel);
  });
}

function updateTracker(schedule, start) {
  const now = new Date();
  const elapsedMinutes = (now.getTime() - start.getTime()) / 60000;
  const totalMinutes = timingConfig.total_minutes;
  const progressPercent = clamp((elapsedMinutes / totalMinutes) * 100, 0, 100);
  const marker = document.getElementById('study-progress-marker');
  if (marker) {
    marker.style.left = `${progressPercent}%`;
  }

  const nowValue = document.getElementById('study-now-time');
  const elapsedValue = document.getElementById('study-elapsed');
  if (nowValue) {
    nowValue.textContent = formatTime(now);
  }
  if (elapsedValue) {
    elapsedValue.textContent = `${Math.max(0, Math.floor(elapsedMinutes))} min`;
  }

  const status = document.getElementById('study-current-status');
  const meta = document.getElementById('study-current-meta');
  let currentNumber = 0;
  let currentBlock = '';

  if (elapsedMinutes < 0) {
    if (status) {
      status.textContent = 'Todavía no empieza el estudio.';
    }
    if (meta) {
      meta.textContent = `Inicio previsto: ${formatTime(start)}.`;
    }
  } else {
    const activeBlock = schedule.blocks.find((block) => now >= block.start && now < block.end);
    const active = schedule.paragraphs.find((row) => now >= row.start && now < row.end);
    if (now >= start && now < schedule.introEnd) {
      currentBlock = 'Apertura';
      if (status) {
        status.textContent = 'Ahora deberías estar en la apertura.';
      }
      if (meta) {
        meta.textContent = `${formatTime(start)} - ${formatTime(schedule.introEnd)}`;
      }
    } else if (active) {
      currentNumber = active.study_number;
      currentBlock = activeBlock?.label || '';
      if (status) {
        status.textContent = `Ahora deberías ir por el párr. ${active.study_number}.`;
      }
      if (meta) {
        meta.textContent = `${formatTime(active.start)} - ${formatTime(active.end)} · ${active.kind}${currentBlock ? ` · ${currentBlock}` : ''}`;
      }
    } else if (now >= schedule.closingStart && now <= schedule.closingEnd) {
      currentBlock = 'Conclusión';
      if (status) {
        status.textContent = 'Ahora deberías estar cerrando el estudio.';
      }
      if (meta) {
        meta.textContent = `${formatTime(schedule.closingStart)} - ${formatTime(schedule.closingEnd)}`;
      }
    } else if (elapsedMinutes > totalMinutes) {
      currentNumber = schedule.paragraphs.at(-1)?.study_number || 0;
      currentBlock = 'Conclusión';
      if (status) {
        status.textContent = 'El tiempo previsto ya terminó.';
      }
      if (meta) {
        meta.textContent = `Cierre previsto: ${formatTime(schedule.closingEnd)}.`;
      }
    }
  }

  setCurrentParagraphState(currentNumber);
  setCurrentBlockState(currentBlock);
}

function applyTimes() {
  const input = document.getElementById('study-start');
  const totalOutput = document.getElementById('study-total-end');
  const value = input.value || timingConfig.default_start;
  const start = new Date(value);

  if (Number.isNaN(start.getTime())) {
    return;
  }

  const schedule = buildSchedule(start);
  const introTarget = document.getElementById('study-opening-range');
  introTarget.textContent = `${formatTime(start)} - ${formatTime(schedule.introEnd)}`;

  for (const row of schedule.paragraphs) {
    const startEl = document.querySelector(`[data-start-for="${row.study_number}"]`);
    if (startEl) {
      startEl.textContent = formatTime(row.start);
    }
    const rangeValue = `${formatTime(row.start)} - ${formatTime(row.end)}`;
    document
      .querySelectorAll(`[data-range-for="${row.study_number}"], [data-range-for="panel-${row.study_number}"]`)
      .forEach((node) => {
        node.textContent = rangeValue;
      });
  }

  const closingRange = document.getElementById('study-closing-range');
  closingRange.textContent = `${formatTime(schedule.closingStart)} - ${formatTime(schedule.closingEnd)}`;
  totalOutput.textContent = formatTime(schedule.closingEnd);

  for (const block of schedule.blocks) {
    const blockEl = document.querySelector(`[data-block-for="${block.label}"]`);
    if (blockEl) {
      blockEl.textContent = `${formatTime(block.start)} - ${formatTime(block.end)}`;
    }
  }

  updateTracker(schedule, start);
}

document.getElementById('study-start').addEventListener('input', applyTimes);
document.getElementById('study-start-now').addEventListener('click', () => {
  const input = document.getElementById('study-start');
  const now = new Date();
  const local = new Date(now.getTime() - now.getTimezoneOffset() * 60000).toISOString().slice(0, 16);
  input.value = local;
  applyTimes();
});

async function toggleFullscreen() {
  if (!document.fullscreenElement) {
    await document.documentElement.requestFullscreen();
  } else {
    await document.exitFullscreen();
  }
}

function updateFullscreenButton() {
  const button = document.getElementById('study-fullscreen');
  if (!button) {
    return;
  }
  button.textContent = document.fullscreenElement ? 'Salir pantalla completa' : 'Pantalla completa';
}

function focusNoteCard(targetId) {
  const target = document.getElementById(targetId);
  if (!target) {
    return;
  }
  document.querySelectorAll('.study-note-card.is-target').forEach((card) => {
    card.classList.remove('is-target');
  });
  target.classList.add('is-target');
  const body = target.querySelector('.study-note-body');
  if (body) {
    body.scrollTo({ top: 0, behavior: 'smooth' });
  }
  target.scrollIntoView({ behavior: 'smooth', block: 'nearest', inline: 'nearest' });
  target.focus({ preventScroll: true });
  window.setTimeout(() => {
    target.classList.remove('is-target');
  }, 1800);
}

document.getElementById('study-fullscreen').addEventListener('click', async () => {
  try {
    await toggleFullscreen();
  } catch (error) {
    console.error('No se pudo cambiar a pantalla completa.', error);
  }
});
document.addEventListener('fullscreenchange', updateFullscreenButton);
document.querySelectorAll('.study-para .parNum[data-note-target]').forEach((marker) => {
  marker.addEventListener('click', () => {
    focusNoteCard(marker.dataset.noteTarget);
  });
  marker.addEventListener('keydown', (event) => {
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      focusNoteCard(marker.dataset.noteTarget);
    }
  });
});

const panel = document.getElementById('study-panel');
const backdrop = document.getElementById('study-panel-backdrop');
document.getElementById('study-open-panel').addEventListener('click', () => {
  panel.hidden = false;
  backdrop.hidden = false;
});
document.getElementById('study-close-panel').addEventListener('click', () => {
  panel.hidden = true;
  backdrop.hidden = true;
});
backdrop.addEventListener('click', () => {
  panel.hidden = true;
  backdrop.hidden = true;
});

bindBibleTooltips();
applyTimes();
updateFullscreenButton();
window.setInterval(applyTimes, 30000);
"""


def slugify(value: str) -> str:
    normalized = unicodedata.normalize("NFKD", value)
    ascii_value = "".join(ch for ch in normalized if not unicodedata.combining(ch))
    ascii_value = ascii_value.lower()
    ascii_value = re.sub(r"[^a-z0-9]+", "-", ascii_value)
    return ascii_value.strip("-")


def md_escape(value: str) -> str:
    return value.replace("\\", "\\\\")


def quote_query(value: str) -> str:
    return f"{WOL_QUERY_BASE}{quote_plus(value)}"


def normalize_query_href(url: str, query_text: str | None = None) -> str:
    raw = absolutize(url)
    if query_text and not raw:
        return quote_query(query_text)
    if query_text and ("/wol/s/" in raw or "/wol/l/" in raw or "?q=" in raw):
        return quote_query(query_text)
    if "?q=" not in raw:
        return raw
    parsed = urlparse(raw)
    values = parse_qs(parsed.query).get("q")
    if not values:
        return raw
    return quote_query(values[0])


def normalize_space(value: str) -> str:
    value = value.replace("\xa0", " ")
    value = re.sub(r"\s+", " ", value)
    return value.strip()


def find_source_file() -> Path:
    if len(sys.argv) > 1:
        source = Path(sys.argv[1]).expanduser().resolve()
        if source.exists():
            return source
        raise SystemExit(f"No existe el archivo fuente: {source}")

    sources = sorted(ROOT.glob("*.mhtml"))
    if not sources:
        raise SystemExit("No encontré ningún archivo .mhtml en la carpeta del proyecto.")
    return sources[0]


def load_notes() -> dict[str, Any]:
    with NOTES_PATH.open("r", encoding="utf-8") as handle:
        return json.load(handle)


def parse_mhtml(source: Path) -> str:
    with source.open("rb") as handle:
        message = BytesParser(policy=policy.default).parse(handle)

    for part in message.walk():
        if part.get_content_type() != "text/html":
            continue
        payload = part.get_payload(decode=True)
        charset = part.get_content_charset() or "utf-8"
        return payload.decode(charset, errors="replace")

    raise SystemExit("No pude encontrar la parte text/html dentro del .mhtml.")


def absolutize(url: str) -> str:
    if not url:
        return url
    if url.startswith("http://") or url.startswith("https://"):
        return url
    if url.startswith("/"):
        return f"https://wol.jw.org{url}"
    return url


def load_bible_tooltip_cache() -> dict[str, Any]:
    if not BIBLE_TOOLTIP_CACHE.exists():
        return {"chapters": {}, "tooltips": {}}
    try:
        cache = json.loads(BIBLE_TOOLTIP_CACHE.read_text(encoding="utf-8"))
    except (json.JSONDecodeError, OSError):
        return {"chapters": {}, "tooltips": {}}
    cache.setdefault("chapters", {})
    cache.setdefault("tooltips", {})
    return cache


def save_bible_tooltip_cache(cache: dict[str, Any]) -> None:
    BIBLE_TOOLTIP_CACHE.write_text(
        json.dumps(cache, ensure_ascii=False, indent=2),
        encoding="utf-8",
    )


def strip_trailing_reference_punctuation(label: str) -> str:
    return normalize_space(label).rstrip(";,.)] ")


def canonicalize_bible_book(book: str) -> str:
    raw = unicodedata.normalize("NFKD", book)
    raw = "".join(ch for ch in raw if not unicodedata.combining(ch))
    raw = raw.lower().replace(".", "")
    raw = re.sub(r"\s+", " ", raw).strip()
    compact = raw.replace(" ", "")
    return BIBLE_BOOK_ALIASES.get(raw) or BIBLE_BOOK_ALIASES.get(compact) or compact


def parse_reference_label(label: str) -> dict[str, Any] | None:
    cleaned = strip_trailing_reference_punctuation(label)
    cleaned = cleaned.replace("\u2011", "-").replace("\u2013", "-").replace("\u2014", "-")
    match = re.match(r"^(?P<book>.+?)\s*(?P<chapter>\d+):(?P<verses>[\d,\- ]+)$", cleaned)
    if not match:
        return None
    book = normalize_space(match.group("book"))
    if not re.search(r"[A-Za-zÁÉÍÓÚÑáéíóúñ]", book):
        return None
    chapter = int(match.group("chapter"))
    verse_spec = normalize_space(match.group("verses"))
    verse_numbers: list[int] = []
    for part in verse_spec.split(","):
        piece = part.strip()
        if not piece:
            continue
        if "-" in piece:
            start_raw, end_raw = piece.split("-", 1)
            start = int(start_raw.strip())
            end = int(end_raw.strip())
            if end < start:
                continue
            verse_numbers.extend(range(start, end + 1))
            continue
        verse_numbers.append(int(piece))
    if not verse_numbers:
        return None
    return {
        "display_title": cleaned,
        "book": book,
        "chapter": chapter,
        "verse_numbers": verse_numbers,
        "chapter_key": f"{canonicalize_bible_book(book)}:{chapter}",
    }


def extract_verse_text(span: Tag) -> str:
    clone = BeautifulSoup(str(span), "lxml").find("span")
    if clone is None:
        return ""
    for anchor in clone.select("a.b, a.fn"):
        anchor.decompose()
    verse_anchor = clone.select_one("a.vl, a.cl")
    if verse_anchor is not None:
        verse_anchor.decompose()
    return normalize_space(clone.get_text(" ", strip=True))


def fetch_bible_chapter_data(href: str, cache: dict[str, Any], chapter_key: str | None = None) -> dict[str, Any] | None:
    href = absolutize(href)
    if chapter_key and chapter_key in cache["chapters"]:
        return cache["chapters"][chapter_key]
    if href in cache["chapters"]:
        return cache["chapters"][href]
    payload = download_bible_chapter_data(href, chapter_key)
    if payload is None:
        return None
    cache["chapters"][href] = payload
    final_url = payload.get("url")
    if final_url:
        cache["chapters"][final_url] = payload
    if chapter_key:
        cache["chapters"][chapter_key] = payload
    return payload


def download_bible_chapter_data(href: str, chapter_key: str | None = None) -> dict[str, Any] | None:
    href = absolutize(href)
    try:
        request = Request(href, headers={"User-Agent": "Mozilla/5.0"})
        with urlopen(request, timeout=30) as response:
            html = response.read().decode("utf-8", errors="ignore")
            final_url = response.geturl()
    except Exception:
        return None

    soup = BeautifulSoup(html, "lxml")
    content_title = ""
    parent_title = ""
    content_title_input = soup.find("input", id="contentTitle")
    parent_title_input = soup.find("input", id="parentTitle")
    if content_title_input is not None:
        content_title = normalize_space(content_title_input.get("value", ""))
    if parent_title_input is not None:
        parent_title = normalize_space(parent_title_input.get("value", ""))

    verses: dict[str, str] = {}
    for span in soup.select("span.v[id]"):
        span_id = span.get("id", "")
        match = re.match(r"v\d+-\d+-(\d+)-\d+$", span_id)
        if not match:
            continue
        verse_number = match.group(1)
        verse_text = extract_verse_text(span)
        if verse_text:
            verses[verse_number] = verse_text

    if not verses:
        return None

    payload = {
        "content_title": content_title,
        "parent_title": parent_title or "La Biblia. Traducción del Nuevo Mundo (edición de estudio)",
        "verses": verses,
        "url": final_url or href,
    }
    return payload


def build_bible_tooltips(article: Tag) -> dict[str, dict[str, Any]]:
    cache = load_bible_tooltip_cache()
    tooltip_map: dict[str, dict[str, Any]] = {}
    assigned: dict[str, str] = {}
    cache_changed = False
    pending_chapters: dict[str, str] = {}
    prepared_links: list[tuple[Tag, str, dict[str, Any]]] = []

    for anchor in article.select("p.themeScrp a.b[href], p.qu a.b[href], p strong a.b[href]"):
        if anchor.find_parent("aside", class_="study-note-card") is not None:
            continue
        label = normalize_space(anchor.get_text(" ", strip=True))
        parsed = parse_reference_label(label)
        if parsed is None:
            continue

        href = absolutize(anchor.get("href", ""))
        if not href:
            continue
        prepared_links.append((anchor, href, parsed))
        if parsed["chapter_key"] not in cache["chapters"]:
            pending_chapters.setdefault(parsed["chapter_key"], href)

    if pending_chapters:
        with ThreadPoolExecutor(max_workers=6) as executor:
            futures = {
                executor.submit(download_bible_chapter_data, href, chapter_key): (chapter_key, href)
                for chapter_key, href in pending_chapters.items()
            }
            for future in as_completed(futures):
                chapter_key, href = futures[future]
                try:
                    payload = future.result()
                except Exception:
                    payload = None
                if payload is None:
                    continue
                cache["chapters"][chapter_key] = payload
                cache["chapters"][href] = payload
                final_url = payload.get("url")
                if final_url:
                    cache["chapters"][final_url] = payload
                cache_changed = True

    for anchor, href, parsed in prepared_links:
        cache_key = f"{href}|{parsed['display_title']}"
        tooltip_key = assigned.get(cache_key)
        if tooltip_key:
            anchor["data-study-tooltip-key"] = tooltip_key
            continue

        tooltip_payload = cache["tooltips"].get(cache_key)
        if tooltip_payload is None:
            chapter = fetch_bible_chapter_data(href, cache, parsed["chapter_key"])
            if chapter is None:
                continue
            cache_changed = True
            verses = []
            for verse_number in parsed["verse_numbers"]:
                verse_text = chapter["verses"].get(str(verse_number))
                if verse_text:
                    verses.append({"number": verse_number, "text": verse_text})
            if not verses:
                continue
            tooltip_payload = {
                "heading": "Pasaje bíblico citado",
                "title": parsed["display_title"],
                "subtitle": chapter["parent_title"],
                "href": href,
                "verses": verses,
            }
            cache["tooltips"][cache_key] = tooltip_payload
            cache_changed = True

        tooltip_key = f"study-bible-tooltip-{len(tooltip_map) + 1}"
        tooltip_map[tooltip_key] = tooltip_payload
        assigned[cache_key] = tooltip_key
        anchor["data-study-tooltip-key"] = tooltip_key
        anchor["data-study-tooltip-trigger"] = "hover"

    if cache_changed:
        save_bible_tooltip_cache(cache)
    return tooltip_map


def extract_source(html: str) -> tuple[BeautifulSoup, Tag, Tag | None, list[str]]:
    soup = BeautifulSoup(html, "lxml")
    article = soup.find("article", id="article")
    if article is None:
        raise SystemExit("No pude localizar <article id='article'> en el HTML extraído.")

    footnote = soup.select_one(".groupFootnote")
    stylesheets = []
    for link in soup.select("head link[rel='stylesheet']"):
        href = link.get("href")
        if href and not href.startswith("cid:"):
            stylesheets.append(href)
    return soup, article, footnote, stylesheets


def clone_fragment(node: Tag | None) -> Tag | None:
    if node is None:
        return None
    return BeautifulSoup(str(node), "lxml").find(node.name)


def make_tag(name: str, attrs: dict[str, Any] | None = None) -> Tag:
    scratch = BeautifulSoup("", "lxml")
    return scratch.new_tag(name, attrs=attrs or {})


def clean_article(article: Tag) -> Tag:
    for selector in [".gen-field", ".dc-screenReaderText", ".pswp", "script"]:
        for bad in article.select(selector):
            bad.decompose()

    for textarea in article.select("textarea"):
        textarea.decompose()

    for label in article.select("label"):
        label.decompose()

    for wrapper in article.select(".documentBanner, #banner"):
        wrapper.decompose()

    return article


def highlight_class(kind: str) -> str:
    mapping = {
        "primary": "study-mark-primary",
        "secondary": "study-mark-secondary",
        "key": "study-mark-key",
        "support": "study-mark-support",
    }
    return mapping.get(kind, "study-mark-primary")


def apply_highlight(paragraph: Tag, snippet: str, kind: str = "primary") -> None:
    for text_node in paragraph.find_all(string=True):
        if not isinstance(text_node, NavigableString):
            continue
        raw_text = str(text_node)
        if snippet in raw_text:
            matched = snippet
        else:
            pattern = re.escape(snippet).replace(r"\ ", r"[ \xa0]+")
            match = re.search(pattern, raw_text)
            if not match:
                continue
            matched = match.group(0)

        full = raw_text
        before, after = full.split(matched, 1)
        mark = make_tag("span", attrs={"class": f"study-mark {highlight_class(kind)}"})
        mark.string = matched
        pieces: list[Tag | NavigableString] = []
        if before:
            pieces.append(NavigableString(before))
        pieces.append(mark)
        if after:
            pieces.append(NavigableString(after))
        for piece in reversed(pieces):
            text_node.insert_after(piece)
        text_node.extract()
        return

    raise SystemExit(
        f"No encontré el fragmento exacto para subrayar en {paragraph.get('id')}: {snippet!r}"
    )


def paragraph_text(tag: Tag | None) -> str:
    if tag is None:
        return ""
    text = tag.get_text(" ", strip=True)
    return normalize_space(text)


def question_map(article: Tag) -> dict[str, str]:
    return {
        tag["id"]: paragraph_text(tag)
        for tag in article.select("p.qu[id]")
        if tag.has_attr("id")
    }


def section_map(article: Tag) -> dict[str, str]:
    return {
        tag["id"]: paragraph_text(tag)
        for tag in article.select("h2[id]")
        if tag.has_attr("id")
    }


def build_paragraph_lookup(article: Tag) -> dict[str, Tag]:
    return {
        tag["id"]: tag
        for tag in article.find_all(id=True)
        if isinstance(tag, Tag) and tag.name == "p"
    }


def build_block_offsets(notes: dict[str, Any]) -> None:
    offset = float(notes["timing"]["intro_minutes"])
    for block in notes["timing"]["blocks"]:
        block["offset_minutes"] = round(offset, 2)
        offset += block["minutes"]


def validate_source(article: Tag, footnote: Tag | None, notes: dict[str, Any]) -> None:
    title = paragraph_text(article.find("h1"))
    if title != notes["meta"]["title"]:
        raise SystemExit(f"El título extraído no coincide: {title!r}")

    study_paragraphs = [
        tag
        for tag in article.select("p[data-rel-pid]")
        if tag.get("id", "").startswith("p") and tag.get("id") not in {"p71", "p73", "p75"}
    ]
    if len(study_paragraphs) != 18:
        raise SystemExit(f"Esperaba 18 párrafos de estudio y encontré {len(study_paragraphs)}.")

    main_sections = [
        tag
        for tag in article.select("h2[id]")
        if tag.get("id") in {"p10", "p16", "p22", "p26"}
    ]
    if len(main_sections) != 4:
        raise SystemExit(f"Esperaba 4 subtítulos principales y encontré {len(main_sections)}.")

    footnotes = [node for node in [footnote] if node is not None]
    if len(footnotes) != 1:
        raise SystemExit(f"Esperaba 1 nota y encontré {len(footnotes)}.")

    total = notes["timing"]["intro_minutes"] + notes["timing"]["closing_minutes"]
    total += sum(item["timing_minutes"] for item in notes["paragraphs"])
    if round(total, 2) != round(notes["timing"]["total_minutes"], 2):
        raise SystemExit(f"El total de minutos no suma {notes['timing']['total_minutes']}: {total}")


def build_notes_index(notes: dict[str, Any]) -> dict[str, dict[str, Any]]:
    return {
        "terms": {slugify(item["term"]): item for item in notes["terms"]},
        "threads": {slugify(item["title"]): item for item in notes["threads"]},
        "scriptures": {slugify(item["ref"]): item for item in notes["scriptures"]},
        "applications": {slugify(item["title"]): item for item in notes.get("applications", [])},
    }


def citation_links(items: list[dict[str, str]], limit: int = 2) -> list[dict[str, str]]:
    links: list[dict[str, str]] = []
    for item in items[:limit]:
        href = item.get("href") or item.get("wol_url")
        label = item.get("label")
        if not href and item.get("query_label"):
            href = quote_query(item["query_label"])
        if not href or not label:
            continue
        links.append(
            {
                "label": label,
                "href": normalize_query_href(href, item.get("query_label")),
            }
        )
    return links


def paragraph_scriptures(notes: dict[str, Any], study_number: int) -> list[dict[str, Any]]:
    priority_order = {"leídos": 0, "adicionales": 1, "complementarios": 2, "otros": 3}
    rows = [
        item
        for item in notes["scriptures"]
        if study_number in item.get("related_paragraphs", [])
    ]
    rows.sort(key=lambda item: (priority_order.get(item["priority"], 99), item["ref"]))
    return rows


def note_insights(
    info: dict[str, Any],
    notes: dict[str, Any],
    index: dict[str, dict[str, Any]],
) -> dict[str, Any]:
    insights: dict[str, Any] = {"term": None, "thread": None, "scriptures": []}
    for link in info.get("note_links", []):
        href = link.get("href", "")
        if "#" not in href:
            continue
        target, anchor = href.split("#", 1)
        if target.endswith("03-terminos-clave.md") and insights["term"] is None:
            term = index["terms"].get(anchor)
            if term:
                insights["term"] = {
                    "title": term["term"],
                    "text": term["definition"],
                    "citations": citation_links(term.get("sources", []), limit=2),
                }
        elif target.endswith("05-relaciones-transversales.md") and insights["thread"] is None:
            thread = index["threads"].get(anchor)
            if thread:
                insights["thread"] = {
                    "title": thread["title"],
                    "text": thread["claim"],
                    "citations": citation_links(thread.get("support", []), limit=2),
                }

    for scripture in paragraph_scriptures(notes, info["study_number"])[:2]:
        insights["scriptures"].append(
            {
                "ref": scripture["ref"],
                "text": scripture["main_relation"],
                "citations": citation_links(
                    [
                        {"label": scripture["ref"], "href": scripture["wol_url"]},
                        {"label": f"{scripture['ref']} (búsqueda)", "href": scripture["query_url"]},
                    ],
                    limit=2,
                ),
            }
        )
    return insights


def append_citation_group(parent: Tag, items: list[dict[str, str]]) -> None:
    if not items:
        return
    group = make_tag("div", attrs={"class": "study-note-citations"})
    for item in items:
        link = make_tag(
            "a",
            attrs={"href": item["href"], "target": "_blank", "rel": "noopener noreferrer"},
        )
        link.string = item["label"]
        group.append(link)
    parent.append(group)


def make_note_section(title: str, text: str, section_class: str, citations: list[dict[str, str]] | None = None) -> Tag:
    section = make_tag("section", attrs={"class": f"study-note-section {section_class}".strip()})
    heading = make_tag("h4")
    heading.string = title
    section.append(heading)
    paragraph = make_tag("p")
    paragraph.string = text
    section.append(paragraph)
    append_citation_group(section, citations or [])
    return section


def create_note_card(
    info: dict[str, Any],
    note_data: dict[str, Any],
) -> Tag:
    note_id = f"study-note-{info['study_number']}"
    aside = make_tag(
        "aside",
        attrs={"class": "study-note-card", "id": note_id, "tabindex": "-1"},
    )

    head = make_tag("div", attrs={"class": "study-note-head"})
    number = make_tag("div", attrs={"class": "study-note-number"})
    number.string = str(info["study_number"])
    head.append(number)

    meta = make_tag("div", attrs={"class": "study-note-meta"})
    badges = make_tag("div", attrs={"class": "study-badges"})
    for label, cls in [
        (info["kind"].capitalize(), ""),
        (f"{format_minutes(info['timing_minutes'])} min", "time"),
        (info.get("start_label", "--"), "note"),
    ]:
        badge = make_tag("span", attrs={"class": f"study-badge {cls}".strip()})
        badge.string = label
        badges.append(badge)
    meta.append(badges)
    head.append(meta)
    aside.append(head)

    body = make_tag("div", attrs={"class": "study-note-body"})
    aside.append(body)

    body.append(make_note_section("Respuesta directa", info["direct_answer"], "response"))
    body.append(make_note_section("Idea troncal", info["main_point"], "thread"))

    concept_question = info.get("concept_question") or info.get("extra_question")
    concept_answer = info.get("concept_answer")
    if concept_question:
        concept = make_tag("section", attrs={"class": "study-note-section concept"})
        title = make_tag("h4")
        title.string = info.get("concept_title", "Concepto clave")
        concept.append(title)
        prompt = make_tag("p", attrs={"class": "study-note-miniquestion"})
        prompt.string = concept_question
        concept.append(prompt)
        answer = make_tag("p")
        answer.string = concept_answer or info["direct_answer"]
        concept.append(answer)
        append_citation_group(concept, info.get("concept_sources", []))
        body.append(concept)

    if note_data.get("term"):
        term = note_data["term"]
        body.append(make_note_section(term["title"], term["text"], "scripture", term["citations"]))

    if note_data.get("thread"):
        thread = note_data["thread"]
        body.append(make_note_section("Relación transversal", thread["text"], "thread", thread["citations"]))

    if note_data.get("scriptures"):
        scripture_section = make_tag("section", attrs={"class": "study-note-section scripture"})
        heading = make_tag("h4")
        heading.string = "Textos citados"
        scripture_section.append(heading)
        for item in note_data["scriptures"]:
            paragraph = make_tag("p")
            strong = make_tag("strong")
            strong.string = f"{item['ref']}: "
            paragraph.append(strong)
            paragraph.append(item["text"])
            scripture_section.append(paragraph)
            append_citation_group(scripture_section, item["citations"])
        body.append(scripture_section)

    body.append(make_note_section("Aplicación", info["application"], "application"))
    return aside


def inject_notes(article: Tag, notes: dict[str, Any]) -> None:
    lookup = build_paragraph_lookup(article)
    note_index = build_notes_index(notes)
    for info in notes["paragraphs"]:
        paragraph = lookup.get(info["article_pid"])
        if paragraph is None:
            raise SystemExit(f"No encontré el párrafo {info['article_pid']} en el artículo.")
        highlights = info.get("highlights")
        if highlights:
            for item in highlights:
                apply_highlight(paragraph, item["text"], item.get("kind", "primary"))
        else:
            for snippet in info.get("underline_text", []):
                apply_highlight(paragraph, snippet, "primary")

        paragraph["data-study-number"] = str(info["study_number"])
        paragraph["class"] = list(paragraph.get("class", [])) + ["study-para"]

        wrapper = make_tag(
            "div",
            attrs={
                "class": f"study-row {info['kind']}",
                "id": f"study-row-{info['study_number']}",
            },
        )
        paragraph.wrap(wrapper)

        par_num = paragraph.select_one(".parNum")
        if par_num is not None:
            par_num["data-note-target"] = f"study-note-{info['study_number']}"
            par_num["tabindex"] = "0"
            par_num["role"] = "button"
            par_num["aria-label"] = f"Ir a las notas del párrafo {info['study_number']}"
            par_num["title"] = f"Ir a las notas del párr. {info['study_number']}"

        note_box = create_note_card(info, note_insights(info, notes, note_index))
        wrapper.append(note_box)

    for question in article.select("p.qu"):
        question["class"] = list(question.get("class", [])) + ["study-question"]


def format_minutes(value: float) -> str:
    if value.is_integer():
        return f"{int(value)}"
    return f"{value:.2f}".rstrip("0").rstrip(".")


def format_html_time(date: datetime) -> str:
    suffix = "a. m." if date.hour < 12 else "p. m."
    hour = date.hour % 12 or 12
    return f"{hour}:{date.minute:02d} {suffix}"


def attach_timing_details(notes: dict[str, Any]) -> None:
    start = datetime.fromisoformat(notes["timing"]["default_start"])
    intro_end = start + timedelta(minutes=notes["timing"]["intro_minutes"])
    notes["timing"]["intro_range_label"] = f"{format_html_time(start)} - {format_html_time(intro_end)}"

    cursor = intro_end
    for paragraph in notes["paragraphs"]:
        end = cursor + timedelta(minutes=paragraph["timing_minutes"])
        paragraph["start_label"] = format_html_time(cursor)
        paragraph["range_label"] = f"{format_html_time(cursor)} - {format_html_time(end)}"
        cursor = end

    closing_end = cursor + timedelta(minutes=notes["timing"]["closing_minutes"])
    notes["timing"]["closing_range_label"] = f"{format_html_time(cursor)} - {format_html_time(closing_end)}"
    notes["timing"]["total_end_label"] = format_html_time(closing_end)

    block_cursor = intro_end
    for block in notes["timing"]["blocks"]:
        block_end = block_cursor + timedelta(minutes=block["minutes"])
        block["range_label"] = f"{format_html_time(block_cursor)} - {format_html_time(block_end)}"
        block_cursor = block_end


def build_timing_rows(notes: dict[str, Any]) -> list[dict[str, str]]:
    start = datetime.fromisoformat(notes["timing"]["default_start"])
    rows: list[dict[str, str]] = []

    cursor = start
    intro_end = cursor + timedelta(minutes=notes["timing"]["intro_minutes"])
    rows.append(
        {
            "label": "Apertura",
            "range": f"{format_html_time(cursor)} - {format_html_time(intro_end)}",
            "minutes": format_minutes(notes["timing"]["intro_minutes"]),
        }
    )
    cursor = intro_end

    for paragraph in notes["paragraphs"]:
        end = cursor + timedelta(minutes=paragraph["timing_minutes"])
        rows.append(
            {
                "label": f"Párr. {paragraph['study_number']}",
                "range": f"{format_html_time(cursor)} - {format_html_time(end)}",
                "minutes": format_minutes(paragraph["timing_minutes"]),
            }
        )
        cursor = end

    end = cursor + timedelta(minutes=notes["timing"]["closing_minutes"])
    rows.append(
        {
            "label": "Conclusión",
            "range": f"{format_html_time(cursor)} - {format_html_time(end)}",
            "minutes": format_minutes(notes["timing"]["closing_minutes"]),
        }
    )
    return rows


def render_control_panel(notes: dict[str, Any]) -> str:
    blocks_html = []
    for block in notes["timing"]["blocks"]:
        blocks_html.append(
            f"<tr data-block-row=\"{block['label']}\"><td>{block['label']}</td><td data-block-for=\"{block['label']}\">{block['range_label']}</td><td>{format_minutes(block['minutes'])}</td></tr>"
        )

    paragraph_rows = "".join(
        f"<tr data-study-row=\"{item['study_number']}\"><td>Párr. {item['study_number']}</td><td data-range-for=\"panel-{item['study_number']}\">{item['range_label']}</td><td>{format_minutes(item['timing_minutes'])}</td></tr>"
        for item in notes["paragraphs"]
    )
    progress_segments = "".join(
        f"<div class=\"study-progress-segment {item['kind']}\" data-study-number=\"{item['study_number']}\" style=\"width:{(item['timing_minutes'] / notes['timing']['total_minutes']) * 100:.4f}%\" title=\"Párr. {item['study_number']} · {format_minutes(item['timing_minutes'])} min\"></div>"
        for item in notes["paragraphs"]
    )
    prompt_items = "".join(
        f"<li><strong>{item['context']}:</strong> {item['phrase']}</li>"
        for item in notes["comment_prompts"][:5]
    )

    return f"""
    <div class="study-top-actions">
      <button id="study-fullscreen" type="button">Pantalla completa</button>
    </div>
    <div class="study-dock" id="study-dock">
      <div class="study-dock-head">
        <div>
          <div class="study-dock-title">{notes['meta']['title']}</div>
          <div class="study-dock-status" id="study-current-status">Ahora deberías ir por el párr. 1.</div>
          <div class="study-dock-meta" id="study-current-meta">Ajusta la hora de inicio en el panel.</div>
        </div>
        <button id="study-open-panel" type="button">Panel</button>
      </div>
      <div class="study-progress">
        <div class="study-progress-track">
          {progress_segments}
          <div class="study-progress-marker" id="study-progress-marker"></div>
        </div>
        <div class="study-progress-scale">
          <span id="study-now-time">--</span>
          <span id="study-elapsed">0 min</span>
          <span>60 min</span>
        </div>
        <div class="study-progress-minutes" aria-hidden="true">
          <span>0</span>
          <span>10</span>
          <span>20</span>
          <span>30</span>
          <span>40</span>
          <span>50</span>
          <span>60</span>
        </div>
      </div>
    </div>
    <div class="study-panel-backdrop" id="study-panel-backdrop" hidden></div>
    <aside class="study-panel" id="study-panel" hidden>
      <div class="study-panel-head">
        <h2>Panel de conducción</h2>
        <button class="secondary" id="study-close-panel" type="button">Cerrar</button>
      </div>
      <div class="study-panel-section">
        <div class="study-controls">
          <div>
            <label for="study-start">Hora real de inicio</label>
            <input id="study-start" type="datetime-local" value="{notes['timing']['default_start']}">
          </div>
          <div>
            <label>Hora estimada de cierre</label>
            <output id="study-total-end">{notes['timing']['total_end_label']}</output>
          </div>
          <div>
            <button class="secondary" id="study-start-now" type="button">Usar hora actual</button>
          </div>
        </div>
      </div>
      <div class="study-panel-section">
        <h3>Tracker</h3>
        <div class="study-current-card">
          <p><strong>Texto temático:</strong> {notes['meta']['theme_scripture']}</p>
          <p><strong>Tema:</strong> {notes['meta']['theme']}</p>
          <p><strong>Apertura:</strong> {notes['meta']['opening_song']}</p>
          <p><strong>Final:</strong> {notes['meta']['closing_song']}</p>
        </div>
      </div>
    <div class="study-panel-section">
      <h3>Bloques</h3>
      <table class="study-timing-table">
        <thead>
          <tr><th>Bloque</th><th>Horario</th><th>Min</th></tr>
        </thead>
        <tbody>
            <tr data-block-row="Apertura"><td>Apertura</td><td id="study-opening-range">{notes['timing']['intro_range_label']}</td><td>{format_minutes(notes['timing']['intro_minutes'])}</td></tr>
            {''.join(blocks_html)}
            <tr data-block-row="Conclusión"><td>Conclusión</td><td id="study-closing-range">{notes['timing']['closing_range_label']}</td><td>{format_minutes(notes['timing']['closing_minutes'])}</td></tr>
        </tbody>
      </table>
    </div>
      <div class="study-panel-section">
        <h3>Párrafos</h3>
        <table class="study-timing-table">
          <thead>
            <tr><th>Párrafo</th><th>Horario</th><th>Min</th></tr>
          </thead>
          <tbody>
            {paragraph_rows}
          </tbody>
        </table>
      </div>
      <div class="study-panel-section">
        <h3>Leyenda de resaltado</h3>
        <div class="study-legend">
          <div class="study-legend-item"><span class="study-legend-swatch" style="background: var(--study-purple)"></span>Respuesta directa</div>
          <div class="study-legend-item"><span class="study-legend-swatch" style="background: var(--study-blue)"></span>Idea secundaria o complementaria</div>
          <div class="study-legend-item"><span class="study-legend-swatch" style="background: var(--study-green)"></span>Idea clave</div>
          <div class="study-legend-item"><span class="study-legend-swatch" style="background: var(--study-pink)"></span>Frase de apoyo</div>
        </div>
      </div>
      <div class="study-panel-section">
        <h3>Frases para animar comentarios</h3>
        <ul>{prompt_items}</ul>
      </div>
    </aside>
    """


def build_html(
    article: Tag,
    footnote: Tag | None,
    stylesheets: list[str],
    notes: dict[str, Any],
    source_name: str,
) -> str:
    base = BeautifulSoup("<!DOCTYPE html><html lang='es'><head></head><body class='study-page'></body></html>", "lxml")
    head = base.head
    body = base.body

    head.append(base.new_tag("meta", charset="utf-8"))
    head.append(base.new_tag("meta", attrs={"name": "viewport", "content": "width=device-width, initial-scale=1"}))
    title = base.new_tag("title")
    title.string = f"{notes['meta']['title']} | Paquete de conducción"
    head.append(title)
    for href in stylesheets:
        link = base.new_tag("link", rel="stylesheet", href=href)
        head.append(link)
    style = base.new_tag("style")
    style.string = LOCAL_CSS
    head.append(style)

    shell = base.new_tag("div", attrs={"class": "study-shell"})
    body.append(shell)

    controls = BeautifulSoup(render_control_panel(notes), "lxml")
    for child in list(controls.body.children):
      if getattr(child, "name", None):
        shell.append(child)

    article_wrap = base.new_tag("section", attrs={"class": "study-layout"})
    shell.append(article_wrap)
    article_card = base.new_tag("div", attrs={"class": "study-article-wrap"})
    article_wrap.append(article_card)

    article_section = base.new_tag("div", attrs={"class": "study-article"})
    article_card.append(article_section)

    article_fragment = clone_fragment(article)
    # Disabled for now to keep builds fast; re-enable when there is time to warm the cache from WOL.
    bible_tooltips = build_bible_tooltips(article_fragment) if ENABLE_REMOTE_BIBLE_TOOLTIPS else {}
    article_section.append(article_fragment)

    tooltip_shell = BeautifulSoup(
        """
        <div class="study-hover-tooltip" id="study-hover-tooltip" hidden>
          <div class="study-hover-tooltip-head">
            <div>
              <p class="study-hover-tooltip-kicker" id="study-hover-tooltip-kicker">Pasaje bíblico citado</p>
              <h3 class="study-hover-tooltip-title"><a id="study-hover-tooltip-title" href="#" rel="noopener noreferrer" target="_blank"></a></h3>
              <p class="study-hover-tooltip-subtitle" id="study-hover-tooltip-subtitle"></p>
            </div>
            <button class="study-hover-tooltip-close" id="study-hover-tooltip-close" type="button" aria-label="Cerrar vista previa">×</button>
          </div>
          <div class="study-hover-tooltip-body" id="study-hover-tooltip-body"></div>
        </div>
        """,
        "lxml",
    )
    body.append(tooltip_shell.body.div)

    timing_data = base.new_tag("script", id="study-timing-data", type="application/json")
    timing_data.string = json.dumps(
        {
            "default_start": notes["timing"]["default_start"],
            "total_minutes": notes["timing"]["total_minutes"],
            "intro_minutes": notes["timing"]["intro_minutes"],
            "closing_minutes": notes["timing"]["closing_minutes"],
            "blocks": notes["timing"]["blocks"],
            "paragraphs": [
                {
                    "study_number": item["study_number"],
                    "kind": item["kind"],
                    "timing_minutes": item["timing_minutes"],
                }
                for item in notes["paragraphs"]
            ],
        },
        ensure_ascii=False,
    )
    body.append(timing_data)
    tooltip_data = base.new_tag("script", id="study-bible-tooltip-data", type="application/json")
    tooltip_data.string = json.dumps(bible_tooltips, ensure_ascii=False)
    body.append(tooltip_data)
    script = base.new_tag("script")
    script.string = LOCAL_JS
    body.append(script)

    return str(base)


def build_article_source_md(
    notes: dict[str, Any],
    article: Tag,
    footnote: Tag | None,
    questions: dict[str, str],
    sections: dict[str, str],
) -> str:
    paragraph_lookup = build_paragraph_lookup(article)
    lines = [
        f"# {notes['meta']['title']}",
        "",
        f"[Abrir en WOL]({notes['meta']['article_url']})",
        "",
        f"**Rango:** {notes['meta']['date_range']}",
        f"**Canción de apertura:** {notes['meta']['opening_song']}",
        f"**Texto temático:** {notes['meta']['theme_scripture']}",
        f"**Tema:** {notes['meta']['theme']}",
        "",
    ]

    current_section = "Introducción"
    section_titles = {
        "p10": sections.get("p10", ""),
        "p16": sections.get("p16", ""),
        "p22": sections.get("p22", ""),
        "p26": sections.get("p26", ""),
    }
    section_markers = {
        4: "p10",
        9: "p16",
        14: "p22",
        17: "p26",
    }

    lines.extend(["## Introducción", ""])

    for info in notes["paragraphs"]:
        marker = section_markers.get(info["study_number"])
        if marker:
            lines.extend([f"## {section_titles[marker]}", ""])
        lines.append(f"### Párr. {info['study_number']}")
        lines.append("")
        lines.append(f"**Pregunta oficial:** {questions[info['question_pid']]}")
        lines.append("")
        lines.append(paragraph_text(paragraph_lookup[info["article_pid"]]))
        lines.append("")

    lines.extend(
        [
            "## ¿Qué respondería?",
            "",
            paragraph_text(article.find(id="p30")),
            "",
            paragraph_text(article.find(id="p32")),
            "",
            paragraph_text(article.find(id="p34")),
            "",
            "## Nota importante",
            "",
            paragraph_text(footnote.find("p") if footnote else None),
            "",
        ]
    )
    return "\n".join(lines)


def relative_doc_link(doc: Path, anchor: str) -> str:
    return f"./{doc.name}#{anchor}"


def build_guide_md(notes: dict[str, Any], article: Tag, questions: dict[str, str]) -> str:
    lines = [
        f"# Guía de conducción: {notes['meta']['title']}",
        "",
        "[Artículo fuente](./00-articulo-fuente.md) | [Textos bíblicos](./02-textos-biblicos.md) | [Términos clave](./03-terminos-clave.md) | [Tiempos](./06-tiempos-conduccion.md)",
        "",
    ]
    lookup = build_paragraph_lookup(article)
    for info in notes["paragraphs"]:
        anchor = f"p{info['study_number']}"
        highlights = info.get("highlights") or [
            {"kind": "primary", "text": snippet} for snippet in info.get("underline_text", [])
        ]
        highlight_line = " / ".join(
            f"{item['kind']}: {item['text']}" for item in highlights
        )
        lines.extend(
            [
                f"## Párr. {info['study_number']} {{#{anchor}}}",
                "",
                f"**Tipo:** {info['kind'].capitalize()}",
                f"**Tiempo sugerido:** {format_minutes(info['timing_minutes'])} min",
                f"**Pregunta oficial:** {questions[info['question_pid']]}",
                f"**Respuesta directa:** {info['direct_answer']}",
                f"**Idea troncal:** {info['main_point']}",
                f"**Aplicación:** {info['application']}",
                f"**Resaltado en el texto:** {highlight_line}",
            ]
        )
        if info.get("extra_question"):
            lines.append(f"**Pregunta adicional:** {info['extra_question']}")
        if info.get("concept_question"):
            lines.append(f"**Concepto clave:** {info.get('concept_title', 'Concepto clave')}")
            lines.append(f"**Pregunta de concepto:** {info['concept_question']}")
            lines.append(f"**Respuesta corta:** {info.get('concept_answer', info['direct_answer'])}")
        lines.append(f"**Texto base del párrafo:** {paragraph_text(lookup[info['article_pid']])}")
        if info.get("note_links"):
            lines.append("**Ampliar:**")
            for link in info["note_links"]:
                lines.append(f"- [{link['label']}]({link['href']})")
        lines.append("")
    return "\n".join(lines)


def ref_key(ref: str) -> str:
    return normalize_space(ref).lower()


def extract_scriptures(
    article: Tag,
    notes: dict[str, Any],
) -> list[dict[str, Any]]:
    paragraph_index = {item["article_pid"]: item["study_number"] for item in notes["paragraphs"]}
    question_index: dict[str, list[int]] = {}
    for item in notes["paragraphs"]:
        question_index.setdefault(item["question_pid"], []).append(item["study_number"])

    manual = {ref_key(item["ref"]): copy.deepcopy(item) for item in notes["scriptures"]}
    extracted: dict[str, dict[str, Any]] = {}

    for anchor in article.select("a.b"):
        ref = paragraph_text(anchor)
        href = absolutize(anchor.get("href", ""))
        if not ref or not href:
            continue
        parent = anchor.find_parent("p")
        if parent is None:
            continue
        key = ref_key(ref)
        if key not in extracted:
            entry = manual.pop(key, None) or {
                "ref": ref,
                "priority": "complementarios",
                "main_relation": "Apoya el desarrollo del artículo y refuerza el tema del rescate.",
                "practical_use": "Úsalo para respaldar un comentario breve si la idea principal ya salió.",
            }
            entry.setdefault("ref", ref)
            entry.setdefault("wol_url", href)
            entry.setdefault("query_url", quote_query(ref))
            entry.setdefault("related_paragraphs", [])
            extracted[key] = entry

        entry = extracted[key]
        entry["wol_url"] = href
        entry.setdefault("query_url", quote_query(ref))

        parent_id = parent.get("id", "")
        related = question_index.get(parent_id, [])
        if parent_id in paragraph_index:
            related.append(paragraph_index[parent_id])
        for paragraph_no in related:
            if paragraph_no not in entry["related_paragraphs"]:
                entry["related_paragraphs"].append(paragraph_no)

        parent_text = paragraph_text(parent).lower()
        if "lea" in parent_text:
            entry["priority"] = "leídos"
        elif "themeScrp" in parent.get("class", []) or "qu" in parent.get("class", []):
            if entry["priority"] == "complementarios":
                entry["priority"] = "adicionales"

    for item in manual.values():
        item.setdefault("query_url", quote_query(item["ref"]))
        item.setdefault("related_paragraphs", [])
        extracted[ref_key(item["ref"])] = item

    return sorted(
        extracted.values(),
        key=lambda item: (
            ["leídos", "adicionales", "complementarios", "otros"].index(item["priority"]),
            item["ref"],
        ),
    )


def build_scriptures_md(notes: dict[str, Any], article: Tag) -> str:
    groups = {
        "leídos": [],
        "adicionales": [],
        "complementarios": [],
        "otros": [],
    }
    for item in extract_scriptures(article, notes):
        groups[item["priority"]].append(item)

    lines = [
        f"# Textos bíblicos: {notes['meta']['title']}",
        "",
        "[Artículo fuente](./00-articulo-fuente.md) | [Guía de conducción](./01-guia-conduccion.md) | [Términos clave](./03-terminos-clave.md)",
        "",
    ]

    for label, heading in [
        ("leídos", "Leídos"),
        ("adicionales", "Adicionales"),
        ("complementarios", "Complementarios"),
        ("otros", "Otros"),
    ]:
        lines.extend([f"## {heading}", ""])
        for item in groups[label]:
            anchor = slugify(item["ref"])
            wol_url = normalize_query_href(item["wol_url"], item.get("query_label"))
            query_url = normalize_query_href(item["query_url"], item["ref"])
            lines.extend(
                [
                    f"### {item['ref']} {{#{anchor}}}",
                    "",
                    f"**Párrafos relacionados:** {', '.join(str(value) for value in item['related_paragraphs']) or 'Sin asignación manual'}",
                    f"**Relación principal:** {item['main_relation']}",
                    f"**Uso para el conductor:** {item['practical_use']}",
                    f"**WOL directo:** [{wol_url}]({wol_url})",
                    f"**Búsqueda abreviada:** [{query_url}]({query_url})",
                    "",
                ]
            )
        if not groups[label]:
            lines.extend(["_Sin entradas adicionales._", ""])

    return "\n".join(lines)


def build_terms_md(notes: dict[str, Any]) -> str:
    lines = [
        f"# Términos clave: {notes['meta']['title']}",
        "",
        "[Artículo fuente](./00-articulo-fuente.md) | [Textos bíblicos](./02-textos-biblicos.md) | [Relaciones transversales](./05-relaciones-transversales.md)",
        "",
    ]
    for item in notes["terms"]:
        anchor = slugify(item["term"])
        lines.extend(
            [
                f"## {item['term']} {{#{anchor}}}",
                "",
                f"**Definición resumida:** {item['definition']}",
                f"**Aporte al tema central:** {item['theme_relation']}",
                f"**Párrafos del artículo:** {', '.join(str(value) for value in item['related_paragraphs'])}",
                f"**Términos relacionados:** {', '.join(item['related_terms'])}",
                "",
                "**Fuentes sugeridas:**",
            ]
        )
        for source in item["sources"]:
            wol_url = normalize_query_href(source["wol_url"], source.get("query_label"))
            query_url = normalize_query_href(source.get("query_url", ""), source["query_label"])
            lines.append(f"- {source['label']}: [directo]({wol_url}) | [búsqueda abreviada]({query_url})")
        lines.append("")
    return "\n".join(lines)


def build_applications_md(notes: dict[str, Any]) -> str:
    lines = [
        f"# Aplicaciones y preguntas: {notes['meta']['title']}",
        "",
        "[Guía de conducción](./01-guia-conduccion.md) | [Relaciones transversales](./05-relaciones-transversales.md) | [Tiempos](./06-tiempos-conduccion.md)",
        "",
        "## Apertura sugerida",
        "",
    ]
    for item in notes["meta"]["opening_questions"]:
        lines.append(f"- {item}")
    lines.extend(["", "## Preguntas adicionales equilibradas", ""])
    for item in notes["paragraphs"]:
        if not item.get("extra_question"):
            continue
        lines.append(f"- Párr. {item['study_number']}: {item['extra_question']}")
    lines.extend(["", "## Frases para animar comentarios", ""])
    for item in notes["comment_prompts"]:
        lines.append(f"- **{item['context']}:** {item['phrase']}")
    lines.extend(["", "## Aplicaciones troncales", ""])
    for item in notes["applications"]:
        lines.append(f"- **{item['title']}:** {item['text']}")
    return "\n".join(lines)


def build_threads_md(notes: dict[str, Any]) -> str:
    lines = [
        f"# Relaciones transversales: {notes['meta']['title']}",
        "",
        "[Términos clave](./03-terminos-clave.md) | [Aplicaciones](./04-aplicaciones-y-preguntas.md)",
        "",
    ]
    for item in notes["threads"]:
        anchor = slugify(item["title"])
        lines.extend(
            [
                f"## {item['title']} {{#{anchor}}}",
                "",
                f"**Planteamiento:** {item['claim']}",
                f"**Conexión con el artículo:** {item['article_connection']}",
                f"**Uso para la conducción:** {item['conductor_use']}",
                "",
                "**Apoyos WOL:**",
            ]
        )
        for source in item["support"]:
            wol_url = normalize_query_href(source["wol_url"], source.get("query_label"))
            query_url = normalize_query_href(source.get("query_url", ""), source["query_label"])
            lines.append(f"- {source['label']}: [directo]({wol_url}) | [búsqueda abreviada]({query_url})")
        lines.append("")
    return "\n".join(lines)


def build_timing_md(notes: dict[str, Any]) -> str:
    rows = build_timing_rows(notes)
    lines = [
        f"# Tiempos de conducción: {notes['meta']['title']}",
        "",
        "[Guía de conducción](./01-guia-conduccion.md) | [HTML principal](./atalaya-rescate-estudio.html)",
        "",
        f"**Inicio base:** {notes['timing']['default_start']}",
        f"**Duración total:** {notes['timing']['total_minutes']} min",
        "",
        "## Cronograma base",
        "",
    ]
    for row in rows:
        lines.append(f"- **{row['label']}:** {row['range']} ({row['minutes']} min)")
    lines.extend(["", "## Bloques principales", ""])
    for block in notes["timing"]["blocks"]:
        lines.append(f"- **{block['label']}:** {format_minutes(block['minutes'])} min")
    lines.extend(["", "## Detalle por párrafo", ""])
    start = datetime.fromisoformat(notes["timing"]["default_start"]) + timedelta(minutes=notes["timing"]["intro_minutes"])
    cursor = start
    for item in notes["paragraphs"]:
        end = cursor + timedelta(minutes=item["timing_minutes"])
        lines.append(
            f"- **Párr. {item['study_number']}:** {format_html_time(cursor)} - {format_html_time(end)} ({format_minutes(item['timing_minutes'])} min, {item['kind']})"
        )
        cursor = end
    return "\n".join(lines)


def main() -> None:
    source_file = find_source_file()
    notes = load_notes()
    build_block_offsets(notes)
    attach_timing_details(notes)
    html = parse_mhtml(source_file)
    _, raw_article, raw_footnote, stylesheets = extract_source(html)
    article = clean_article(clone_fragment(raw_article))
    footnote = clone_fragment(raw_footnote)
    validate_source(article, footnote, notes)

    questions = question_map(article)
    sections = section_map(article)
    inject_notes(article, notes)

    OUTPUT_HTML.write_text(
        build_html(article, footnote, stylesheets, notes, source_file.name),
        encoding="utf-8",
    )
    OUTPUT_FILES["source"].write_text(
        build_article_source_md(notes, article, footnote, questions, sections),
        encoding="utf-8",
    )
    OUTPUT_FILES["guide"].write_text(build_guide_md(notes, article, questions), encoding="utf-8")
    OUTPUT_FILES["scriptures"].write_text(build_scriptures_md(notes, article), encoding="utf-8")
    OUTPUT_FILES["terms"].write_text(build_terms_md(notes), encoding="utf-8")
    OUTPUT_FILES["applications"].write_text(build_applications_md(notes), encoding="utf-8")
    OUTPUT_FILES["threads"].write_text(build_threads_md(notes), encoding="utf-8")
    OUTPUT_FILES["timing"].write_text(build_timing_md(notes), encoding="utf-8")

    print(f"Paquete generado a partir de: {source_file.name}")
    print(f"HTML: {OUTPUT_HTML.name}")
    for path in OUTPUT_FILES.values():
        print(path.name)


if __name__ == "__main__":
    main()
