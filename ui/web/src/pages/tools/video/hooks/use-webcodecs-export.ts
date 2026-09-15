import { useCallback, useRef, useState } from "react";
import { drawStoryboardFrame } from "../components/render-shared";

// ── Types ──

interface KenBurns {
  zoom_from: number;
  zoom_to: number;
  pan: "none" | "left" | "right" | "up" | "down";
}
interface Caption {
  text: string;
  position?: "top" | "center" | "bottom";
  font_size?: number;
}
interface Scene {
  type: "image" | "video" | "color";
  source?: string;
  color?: string;
  duration_sec: number;
  fit?: "cover" | "contain";
  mute?: boolean;
  ken_burns?: KenBurns;
  caption?: Caption;
  narration?: string;
}
interface Storyboard {
  version: number;
  canvas: { width: number; height: number; fps: number };
  scenes: Scene[];
  audio?: { bgm_path?: string; bgm_volume?: number };
  output?: { format?: string; height?: number };
}

export type ExportMethod = "browser-fast" | "browser-quality" | "server";

export interface UseWebCodecsExportReturn {
  isSupported: boolean;
  exportMethod: ExportMethod;
  setExportMethod: (m: ExportMethod) => void;
  exportWebCodecs: (
    sb: Storyboard,
    onProgress?: (p: number) => void,
  ) => Promise<Blob>;
  isExporting: boolean;
  progress: number;
  isOverloaded: boolean;
  cancel: () => void;
}

// ── WebCodecs support check ──

function checkWebCodecsSupport(): boolean {
  try {
    return (
      typeof VideoEncoder !== "undefined" &&
      typeof VideoDecoder !== "undefined" &&
      typeof ImageBitmap !== "undefined"
    );
  } catch {
    return false;
  }
}

// ── Image loading helper ──

function loadImage(src: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image();
    img.crossOrigin = "anonymous";
    img.onload = () => resolve(img);
    img.onerror = () => reject(new Error(`Failed to load: ${src}`));
    img.src = src;
  });
}

// ── Scene helpers ──

function totalDuration(scenes: Scene[]): number {
  return scenes.reduce((acc, s) => acc + (Number(s.duration_sec) || 0), 0);
}

function sceneAtTime(
  scenes: Scene[],
  time: number,
): { index: number; localTime: number } {
  let elapsed = 0;
  for (let i = 0; i < scenes.length; i++) {
    const dur = Number(scenes[i]?.duration_sec) || 0;
    if (time < elapsed + dur || i === scenes.length - 1) {
      return { index: i, localTime: time - elapsed };
    }
    elapsed += dur;
  }
  return { index: 0, localTime: 0 };
}

// ── Minimal MP4 muxer (isom/avc1) ──
//
// Produces a valid MP4 file from raw H.264 Annex-B encoded chunks.
// Structure: ftyp + moov (mvhd + trak[tkhd, mdia[hdlr, minf, stbl]]) + mdat.

function u32(n: number): Uint8Array {
  const b = new Uint8Array(4);
  new DataView(b.buffer).setUint32(0, n);
  return b;
}

function u16(n: number): Uint8Array {
  const b = new Uint8Array(2);
  new DataView(b.buffer).setUint16(0, n);
  return b;
}

function box(type: string, payload: Uint8Array): Uint8Array {
  const size = 8 + payload.length;
  const header = new Uint8Array(8);
  new DataView(header.buffer).setUint32(0, size);
  header[4] = type.charCodeAt(0);
  header[5] = type.charCodeAt(1);
  header[6] = type.charCodeAt(2);
  header[7] = type.charCodeAt(3);
  const out = new Uint8Array(size);
  out.set(header, 0);
  out.set(payload, 8);
  return out;
}

function boxes(...parts: Uint8Array[]): Uint8Array {
  let total = 0;
  for (const p of parts) total += p.length;
  const out = new Uint8Array(total);
  let offset = 0;
  for (const p of parts) {
    out.set(p, offset);
    offset += p.length;
  }
  return out;
}

/** Convert H.264 Annex-B bytes to AVCC format for the stsd box. */
function annexBToAVCC(chunks: Uint8Array[]): Uint8Array {
  // Find SPS and PPS from the first keyframe chunk
  let sps: Uint8Array | null = null;
  let pps: Uint8Array | null = null;

  for (const chunk of chunks) {
    const nalus = parseAnnexB(chunk);
    for (const nalu of nalus) {
      if (nalu.length === 0) continue;
      const type = nalu[0]! & 0x1f;
      if (type === 7 && sps === null) sps = nalu;
      if (type === 8 && pps === null) pps = nalu;
    }
    if (sps && pps) break;
  }

  // Build avcC box
  if (!sps || !pps) {
    // Fallback: minimal SPS/PPS
    sps = new Uint8Array([0x67, 0x42, 0x00, 0x0a, 0xe9, 0x40, 0x50, 0x1e, 0xd0, 0x80, 0x00, 0x00, 0x03, 0x00, 0x80, 0x00, 0x00, 0x0f, 0x40, 0x00, 0x05, 0x06, 0xe0, 0x0c, 0x8b, 0xc0, 0x20]);
    pps = new Uint8Array([0x68, 0xce, 0x38, 0x80]);
  }

  const avccParts: Uint8Array[] = [
    new Uint8Array([0x01]), // configurationVersion
    sps.subarray(1, 2),     // AVCProfileIndication
    sps.subarray(2, 3),     // profile_compatibility
    sps.subarray(3, 4),     // AVCLevelIndication
    new Uint8Array([0xff]),  // lengthSizeMinusOne = 3 (4 bytes NALU length)
    new Uint8Array([0xe1]),  // numOfSequenceParameterSets = 1
    u16(sps.length),
    sps,
    new Uint8Array([0x01]),  // numOfPictureParameterSets = 1
    u16(pps.length),
    pps,
  ];

  let totalLen = 0;
  for (const p of avccParts) totalLen += p.length;
  const avccPayload = new Uint8Array(totalLen);
  let off = 0;
  for (const p of avccParts) {
    avccPayload.set(p, off);
    off += p.length;
  }

  return box("avcC", avccPayload);
}

function parseAnnexB(data: Uint8Array): Uint8Array[] {
  const nalus: Uint8Array[] = [];
  let i = 0;
  while (i < data.length - 3) {
    // Find start code (0x00000001 or 0x000001)
    let startCodeLen = 0;
    if (data[i] === 0 && data[i + 1] === 0) {
      if (data[i + 2] === 0 && data[i + 3] === 1) {
        startCodeLen = 4;
      } else if (data[i + 2] === 1) {
        startCodeLen = 3;
      }
    }
    if (startCodeLen > 0) {
      const start = i + startCodeLen;
      // Find next start code
      let end = data.length;
      for (let j = start + 1; j < data.length - 3; j++) {
        if (
          data[j] === 0 &&
          data[j + 1] === 0 &&
          (data[j + 2] === 1 || (data[j + 2] === 0 && data[j + 3] === 1))
        ) {
          end = j;
          break;
        }
      }
      nalus.push(data.subarray(start, end));
      i = end;
    } else {
      i++;
    }
  }
  return nalus;
}

/** Convert Annex-B chunk to length-prefixed format for MP4 mdat. */
function annexBToLengthPrefixed(chunk: Uint8Array): Uint8Array {
  const nalus = parseAnnexB(chunk);
  let totalLen = 0;
  for (const nalu of nalus) totalLen += 4 + nalu.length;
  const out = new Uint8Array(totalLen);
  let off = 0;
  for (const nalu of nalus) {
    new DataView(out.buffer, off).setUint32(0, nalu.length);
    out.set(nalu, off + 4);
    off += 4 + nalu.length;
  }
  return out;
}

interface MuxInput {
  chunks: Uint8Array[];
  timestamps: number[]; // in seconds
  isKeyframe: boolean[];
  width: number;
  height: number;
  fps: number;
}

function muxMP4(input: MuxInput): Blob {
  const { chunks, timestamps, width, height, fps } = input;
  const timescale = 90000;

  // Build mdat (concatenated length-prefixed NALUs)
  const mdatParts: Uint8Array[] = [];
  let mdatSize = 0;
  for (const chunk of chunks) {
    const prefixed = annexBToLengthPrefixed(chunk);
    mdatParts.push(prefixed);
    mdatSize += prefixed.length;
  }
  const mdatPayload = new Uint8Array(mdatSize);
  let mdatOff = 0;
  for (const p of mdatParts) {
    mdatPayload.set(p, mdatOff);
    mdatOff += p.length;
  }
  const mdatBox = box("mdat", mdatPayload);

  // Build stts (time-to-sample): one entry per sample
  const sttsData = new Uint8Array(4 + chunks.length * 8); // count + entries
  new DataView(sttsData.buffer).setUint32(0, chunks.length);
  const frameDuration = Math.round(timescale / fps);
  for (let i = 0; i < chunks.length; i++) {
    const entryOff = 4 + i * 8;
    new DataView(sttsData.buffer).setUint32(entryOff, 1); // sample_count
    new DataView(sttsData.buffer).setUint32(entryOff + 4, frameDuration); // sample_delta
  }
  const sttsBox = box("stts", sttsData);

  // Build stss (sync samples) - mark keyframes
  const keyframeIndices: number[] = [];
  for (let i = 0; i < input.isKeyframe.length; i++) {
    if (input.isKeyframe[i]) keyframeIndices.push(i + 1); // 1-based
  }
  const stssData = new Uint8Array(4 + keyframeIndices.length * 4);
  new DataView(stssData.buffer as ArrayBuffer).setUint32(0, keyframeIndices.length);
  for (let i = 0; i < keyframeIndices.length; i++) {
    new DataView(stssData.buffer as ArrayBuffer).setUint32(4 + i * 4, keyframeIndices[i]!);
  }
  const stssBox = box("stss", stssData);

  // Build stsz (sample sizes)
  const stszData = new Uint8Array(8 + chunks.length * 4);
  new DataView(stszData.buffer).setUint32(4, chunks.length);
  for (let i = 0; i < chunks.length; i++) {
    new DataView(stszData.buffer).setUint32(8 + i * 4, mdatParts[i]!.length);
  }
  const stszBox = box("stsz", stszData);

  // Build stco (chunk offsets) - one chunk per sample
  // We need to calculate offsets relative to mdat payload start
  const stcoData = new Uint8Array(4 + chunks.length * 4);
  new DataView(stcoData.buffer).setUint32(0, chunks.length);
  let sampleOffset = 0;
  for (let i = 0; i < chunks.length; i++) {
    // offset is from file start; mdat header is 8 bytes
    // We'll calculate after we know moov size - use placeholder
    new DataView(stcoData.buffer).setUint32(4 + i * 4, sampleOffset);
    sampleOffset += mdatParts[i]!.length;
  }
  const stcoBox = box("stco", stcoData);

  // Build avcC
  const avcc = annexBToAVCC(chunks);

  // Build avc1 (sample entry)
  const avc1Payload = boxes(
    new Uint8Array(6), // reserved
    u16(1), // data_reference_index
    new Uint8Array(2), // pre_defined + reserved
    new Uint8Array(12), // pre_defined[3] + reserved[3]
    u16(width),
    u16(height),
    u32(0x00480000), // horizresolution (72 dpi)
    u32(0x00480000), // vertresolution (72 dpi)
    u32(0), // reserved
    u16(1), // frame_count
    new Uint8Array(32), // compressorname
    u16(0x0018), // depth
    u16(0xffff), // pre_defined = -1
    avcc,
  );
  const avc1Box = box("avc1", avc1Payload);

  // Build stsd
  const stsdPayload = new Uint8Array(4 + avc1Box.length); // version/flags + entry
  new DataView(stsdPayload.buffer).setUint32(0, 0); // version/flags
  stsdPayload.set(avc1Box, 4);
  const stsdBox = box("stsd", stsdPayload);

  // Build stbl
  const stblBox = box(
    "stbl",
    boxes(stsdBox, sttsBox, stssBox, stszBox, stcoBox),
  );

  // Build dinf/dref
  const drefEntry = box("url ", new Uint8Array([0x00, 0x00, 0x00, 0x01])); // self-contained flag
  const drefPayload = new Uint8Array(4 + drefEntry.length);
  new DataView(drefPayload.buffer).setUint32(0, 0);
  drefPayload.set(drefEntry, 4);
  const drefBox = box("dref", drefPayload);
  const dinfBox = box("dinf", drefBox);

  // Build vmhd
  const vmhdPayload = new Uint8Array(8); // version + flags + graphicsmode + opcolor
  new DataView(vmhdPayload.buffer).setUint16(6, 0x0040); // graphicsmode = copy
  const vmhdBox = box("vmhd", vmhdPayload);

  // Build minf
  const minfBox = box("minf", boxes(vmhdBox, dinfBox, stblBox));

  // Build hdlr
  const hdlrPayload = boxes(
    new Uint8Array(4), // version + flags
    new Uint8Array([0, 0, 0, 0]), // pre_defined
    new Uint8Array([
      0x76, 0x69, 0x64, 0x65, // "vide"
      0x00, 0x00, 0x00, 0x00,
      0x00, 0x00, 0x00, 0x00,
      0x00, 0x00, 0x00, 0x00,
    ]),
    new Uint8Array(4), // reserved
    u16(0), // pre_defined
  );
  const hdlrBox = box("hdlr", hdlrPayload);

  // Build mdia
  const mdiaBox = box("mdia", boxes(hdlrBox, minfBox));

  // Build tkhd
  const tkhdPayload = new Uint8Array(84);
  const tkhdDV = new DataView(tkhdPayload.buffer);
  tkhdDV.setUint8(0, 0); // version
  tkhdDV.setUint32(4, 0x00000001); // flags = enabled
  tkhdDV.setUint32(8, 0); // creation_time
  tkhdDV.setUint32(12, 0); // modification_time
  tkhdDV.setUint32(16, 1); // track_ID
  tkhdDV.setUint32(20, 0); // reserved
  const duration = timestamps.length > 0
    ? Math.round((timestamps[timestamps.length - 1]! + 1 / fps) * timescale)
    : 0;
  tkhdDV.setUint32(24, duration); // duration
  tkhdDV.setUint32(28, 0); // reserved[2]
  tkhdDV.setUint32(32, 0); // reserved[3]
  tkhdDV.setUint32(36, 0); // layer
  tkhdDV.setUint32(40, 0); // alternate_group
  tkhdDV.setUint32(44, 0); // volume (0 for video)
  tkhdDV.setUint32(48, 0); // reserved
  // Matrix (identity): 0x00010000, 0, 0, 0x00010000, 0, 0
  tkhdDV.setUint32(52, 0x00010000);
  tkhdDV.setUint32(60, 0x00010000);
  tkhdDV.setUint32(72, width << 16); // width (fixed point 16.16)
  tkhdDV.setUint32(76, height << 16); // height (fixed point 16.16)
  const tkhdBox = box("tkhd", tkhdPayload);

  // Build trak
  const trakBox = box("trak", boxes(tkhdBox, mdiaBox));

  // Build mvhd
  const mvhdPayload = new Uint8Array(108);
  const mvhdDV = new DataView(mvhdPayload.buffer);
  mvhdDV.setUint8(0, 0); // version
  mvhdDV.setUint32(4, 0); // flags
  mvhdDV.setUint32(8, 0); // creation_time
  mvhdDV.setUint32(12, 0); // modification_time
  mvhdDV.setUint32(16, timescale); // timescale
  mvhdDV.setUint32(20, duration); // duration
  mvhdDV.setUint32(24, 0x00010000); // rate = 1.0
  mvhdDV.setUint16(28, 0x0100); // volume = 1.0
  // reserved (10 bytes at offset 30)
  // Matrix (identity at offset 40)
  mvhdDV.setUint32(40, 0x00010000);
  mvhdDV.setUint32(56, 0x00010000);
  mvhdDV.setUint32(72, 0x00010000);
  // next_track_ID
  mvhdDV.setUint32(96, 2);
  const mvhdBox = box("mvhd", mvhdPayload);

  // Build moov
  const moovBox = box("moov", boxes(mvhdBox, trakBox));

  // Build ftyp
  const ftypPayload = boxes(
    new Uint8Array([
      0x69, 0x73, 0x6f, 0x6d, // "isom"
    ]),
    u32(0x200), // version
    new Uint8Array([
      0x69, 0x73, 0x6f, 0x6d, // "isom"
      0x61, 0x76, 0x63, 0x31, // "avc1"
      0x69, 0x73, 0x6f, 0x6d, // "isom"
    ]),
  );
  const ftypBox = box("ftyp", ftypPayload);

  // Now fix stco offsets: moov size + ftyp size + 8 (mdat header)
  const moovSize = moovBox.length;
  const ftypSize = ftypBox.length;
  const baseOffset = ftypSize + moovSize + 8; // mdat header

  // Rebuild stco with correct offsets
  const fixedStcoData = new Uint8Array(4 + chunks.length * 4);
  new DataView(fixedStcoData.buffer).setUint32(0, chunks.length);
  sampleOffset = 0;
  for (let i = 0; i < chunks.length; i++) {
    new DataView(fixedStcoData.buffer).setUint32(
      4 + i * 4,
      baseOffset + sampleOffset,
    );
    sampleOffset += mdatParts[i]!.length;
  }
  const fixedStcoBox = box("stco", fixedStcoData);

  // Rebuild stbl with fixed stco
  const fixedStblBox = box(
    "stbl",
    boxes(stsdBox, sttsBox, stssBox, stszBox, fixedStcoBox),
  );
  const fixedMinfBox = box("minf", boxes(vmhdBox, dinfBox, fixedStblBox));
  const fixedMdiaBox = box("mdia", boxes(hdlrBox, fixedMinfBox));
  const fixedTrakBox = box("trak", boxes(tkhdBox, fixedMdiaBox));
  const fixedMoovBox = box("moov", boxes(mvhdBox, fixedTrakBox));

  // .slice() detaches from any SharedArrayBuffer view, giving a plain ArrayBuffer
  return new Blob(
    [ftypBox.slice().buffer, fixedMoovBox.slice().buffer, mdatBox.slice().buffer],
    { type: "video/mp4" },
  );
}

// ── WebCodecs export (faster-than-realtime) ──

/** Overload detection: tracks whether encoding is keeping up.
 *  If the encoder queue grows beyond QUEUE_HIGH_WATERMARK or if wall-clock
 *  time spent encoding exceeds the video timeline, we declare overload
 *  and the caller should fall back to server export. */
const QUEUE_HIGH_WATERMARK = 10;
const OVERLOAD_RATIO = 2.0; // if encoding takes > 2x video duration, overloaded

async function exportWithWebCodecs(
  storyboard: Storyboard,
  onProgress?: (pct: number) => void,
  signal?: AbortSignal,
): Promise<{ blob: Blob; overloaded: boolean }> {
  const { width, height, fps } = storyboard.canvas;
  const totalSec = totalDuration(storyboard.scenes);
  if (totalSec <= 0) throw new Error("Storyboard has no duration");

  // Canvas for frame rendering
  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = height;
  const ctx = canvas.getContext("2d");
  if (!ctx) throw new Error("Canvas 2D context unavailable");

  // Reusable scratch canvases for transitions
  const scratchA = document.createElement("canvas");
  scratchA.width = width;
  scratchA.height = height;
  const scratchB = document.createElement("canvas");
  scratchB.width = width;
  scratchB.height = height;

  // Preload images
  const imageCache = new Map<string, HTMLImageElement>();
  for (const scene of storyboard.scenes) {
    if ((scene.type === "image" || scene.type === "video") && scene.source) {
      if (!imageCache.has(scene.source)) {
        try {
          const img = await loadImage(scene.source);
          imageCache.set(scene.source, img);
        } catch {
          // Skip failed images
        }
      }
    }
  }

  const drawFrame = (time: number) => {
    const { index, localTime } = sceneAtTime(storyboard.scenes, time);
    drawStoryboardFrame(
      ctx,
      canvas,
      storyboard.scenes,
      index,
      localTime,
      imageCache,
      scratchA,
      scratchB,
    );
  };

  // VideoEncoder setup
  const encodedChunks: Uint8Array[] = [];
  const frameTimestamps: number[] = [];
  const isKeyframeFlags: boolean[] = [];
  const encodeStart = performance.now();
  const videoDurationMs = totalSec * 1000;

  const encoder = new VideoEncoder({
    output: (chunk, _metadata) => {
      const data = new Uint8Array(chunk.byteLength);
      chunk.copyTo(data);
      encodedChunks.push(data);
      frameTimestamps.push(chunk.timestamp / 1_000_000);
      isKeyframeFlags.push(chunk.type === "key");
    },
    error: (e) => {
      throw new Error(`VideoEncoder error: ${e.message}`);
    },
  });

  encoder.configure({
    codec: "avc1.42001f", // H.64 Baseline Level 3.1
    width,
    height,
    bitrate: 2_000_000,
    framerate: fps,
    avc: { format: "annexb" },
  });

  // Encode frames
  const totalFrames = Math.ceil(totalSec * fps);
  const frameDurationUs = 1_000_000 / fps;

  for (let frameNum = 0; frameNum < totalFrames; frameNum++) {
    if (signal?.aborted) {
      encoder.close();
      throw new Error("Export cancelled");
    }

    const time = frameNum / fps;
    drawFrame(time);

    // Get ImageBitmap from canvas for efficient transfer
    const bitmap = await createImageBitmap(canvas);
    const frame = new VideoFrame(bitmap, {
      timestamp: Math.round(frameNum * frameDurationUs),
      duration: Math.round(frameDurationUs),
    });
    bitmap.close();

    // Overload detection: check if encoder queue is too deep
    const queueSize = encoder.encodeQueueSize;
    if (queueSize > QUEUE_HIGH_WATERMARK) {
      encoder.close();
      return { blob: new Blob(), overloaded: true };
    }

    // Encode (keyframe every 30 frames)
    const isKeyframe = frameNum % 30 === 0;
    encoder.encode(frame, { keyFrame: isKeyframe });
    frame.close();

    // Progress
    if (onProgress) {
      onProgress(Math.min(99, Math.round(((frameNum + 1) / totalFrames) * 100)));
    }
  }

  // Flush encoder
  await encoder.flush();
  encoder.close();

  // Check wall-clock overload
  const elapsedMs = performance.now() - encodeStart;
  if (elapsedMs > videoDurationMs * OVERLOAD_RATIO) {
    return { blob: new Blob(), overloaded: true };
  }

  if (encodedChunks.length === 0) {
    throw new Error("No frames encoded");
  }

  // Mux into MP4
  const blob = muxMP4({
    chunks: encodedChunks,
    timestamps: frameTimestamps,
    isKeyframe: isKeyframeFlags,
    width,
    height,
    fps,
  });

  if (onProgress) onProgress(100);
  return { blob, overloaded: false };
}

// ── Hook ──

export function useWebCodecsExport(): UseWebCodecsExportReturn {
  const [isExporting, setIsExporting] = useState(false);
  const [progress, setProgress] = useState(0);
  const [exportMethod, setExportMethod] = useState<ExportMethod>("browser-fast");
  const [isOverloaded, setIsOverloaded] = useState(false);
  const abortRef = useRef<AbortController | null>(null);

  const isSupported = checkWebCodecsSupport();

  const exportWebCodecs = useCallback(
    async (
      sb: Storyboard,
      onProgress?: (p: number) => void,
    ): Promise<Blob> => {
      const controller = new AbortController();
      abortRef.current = controller;
      setIsExporting(true);
      setProgress(0);
      setIsOverloaded(false);

      try {
        const result = await exportWithWebCodecs(
          sb,
          (p) => {
            setProgress(p);
            onProgress?.(p);
          },
          controller.signal,
        );

        if (result.overloaded) {
          setIsOverloaded(true);
          throw new Error("OVERLOADED");
        }

        return result.blob;
      } finally {
        setIsExporting(false);
        abortRef.current = null;
      }
    },
    [],
  );

  const cancel = useCallback(() => {
    abortRef.current?.abort();
    abortRef.current = null;
    setIsExporting(false);
    setProgress(0);
    setIsOverloaded(false);
  }, []);

  return {
    isSupported,
    exportMethod,
    setExportMethod,
    exportWebCodecs,
    isExporting,
    progress,
    isOverloaded,
    cancel,
  };
}
