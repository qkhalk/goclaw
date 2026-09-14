import { useCallback, useRef, useState } from "react";

/**
 * Mask region that the user can position over the watermark area.
 * Coordinates are in percentages of the video dimensions (0-100).
 */
export interface MaskRegion {
  x: number; // center x in % of video width
  y: number; // center y in % of video height
  width: number; // width in % of video width
  height: number; // height in % of video height
  blurRadius: number; // CSS blur radius in px for the canvas filter
}

export const DEFAULT_MASK: MaskRegion = {
  x: 50,
  y: 50,
  width: 30,
  height: 10,
  blurRadius: 12,
};

export const MAX_VIDEO_DURATION_S = 30;
export const MAX_VIDEO_SIZE_BYTES = 100 * 1024 * 1024; // 100 MB

interface UseVideoWatermarkReturn {
  process: (file: File, mask?: MaskRegion) => Promise<void>;
  progress: { current: number; total: number } | null;
  outputUrl: string | null;
  outputName: string | null;
  isProcessing: boolean;
  cancel: () => void;
  error: string | null;
}

export function useVideoWatermark(): UseVideoWatermarkReturn {
  const [progress, setProgress] = useState<{ current: number; total: number } | null>(null);
  const [outputUrl, setOutputUrl] = useState<string | null>(null);
  const [outputName, setOutputName] = useState<string | null>(null);
  const [isProcessing, setIsProcessing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const abortRef = useRef(false);
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const prevUrlRef = useRef<string | null>(null);

  const cancel = useCallback(() => {
    abortRef.current = true;
  }, []);

  const process = useCallback(
    async (file: File, mask: MaskRegion = DEFAULT_MASK) => {
      // Validate file
      if (!file.type.startsWith("video/")) {
        setError("Not a video file");
        return;
      }
      if (file.size > MAX_VIDEO_SIZE_BYTES) {
        setError(`File too large. Maximum is ${MAX_VIDEO_SIZE_BYTES / (1024 * 1024)}MB`);
        return;
      }

      abortRef.current = false;
      setError(null);
      setOutputUrl(null);
      setOutputName(null);
      setIsProcessing(true);
      setProgress(null);

      // Cleanup previous output URL
      if (prevUrlRef.current) {
        URL.revokeObjectURL(prevUrlRef.current);
        prevUrlRef.current = null;
      }

      try {
        const videoUrl = URL.createObjectURL(file);
        const video = document.createElement("video");
        video.crossOrigin = "anonymous";
        video.muted = true;
        video.preload = "auto";
        video.src = videoUrl;
        videoRef.current = video;

        // Wait for metadata
        await new Promise<void>((resolve, reject) => {
          video.onloadedmetadata = () => resolve();
          video.onerror = () => reject(new Error("Failed to load video"));
          setTimeout(() => reject(new Error("Video load timeout")), 10000);
        });

        // Check duration
        if (video.duration > MAX_VIDEO_DURATION_S) {
          URL.revokeObjectURL(videoUrl);
          setError(`Video too long. Maximum is ${MAX_VIDEO_DURATION_S} seconds`);
          setIsProcessing(false);
          return;
        }

        const vw = video.videoWidth;
        const vh = video.videoHeight;

        // Create offscreen canvas
        const canvas = document.createElement("canvas");
        canvas.width = vw;
        canvas.height = vh;
        const ctx = canvas.getContext("2d");
        if (!ctx) {
          URL.revokeObjectURL(videoUrl);
          setError("Canvas 2D not supported");
          setIsProcessing(false);
          return;
        }

        // Capture stream from canvas for MediaRecorder
        const fps = 30;
        const canvasStream = canvas.captureStream(fps);
        // Use WebM container via MediaRecorder
        const mimeType = MediaRecorder.isTypeSupported("video/webm;codecs=vp9")
          ? "video/webm;codecs=vp9"
          : MediaRecorder.isTypeSupported("video/webm;codecs=vp8")
            ? "video/webm;codecs=vp8"
            : "video/webm";

        const recorder = new MediaRecorder(canvasStream, {
          mimeType,
          videoBitsPerSecond: 5_000_000,
        });

        const chunks: Blob[] = [];
        recorder.ondataavailable = (e) => {
          if (e.data.size > 0) chunks.push(e.data);
        };

        const recorderDone = new Promise<Blob>((resolve, reject) => {
          recorder.onstop = () => {
            const blob = new Blob(chunks, { type: mimeType });
            resolve(blob);
          };
          recorder.onerror = () => reject(new Error("MediaRecorder error"));
        });

        // Calculate mask pixel coordinates
        const mx = Math.round((mask.x / 100) * vw);
        const my = Math.round((mask.y / 100) * vh);
        const mw = Math.round((mask.width / 100) * vw);
        const mh = Math.round((mask.height / 100) * vh);
        const mx0 = Math.max(0, mx - mw / 2);
        const my0 = Math.max(0, my - mh / 2);
        const mx1 = Math.min(vw, mx + mw / 2);
        const my1 = Math.min(vh, my + mh / 2);

        // Estimate total frames
        const totalFrames = Math.ceil(video.duration * fps);
        setProgress({ current: 0, total: totalFrames });

        // Seek to start
        video.currentTime = 0;
        await new Promise<void>((resolve) => {
          video.onseeked = () => resolve();
        });

        // Start recording
        recorder.start();

        // Process frames
        let frame = 0;

        // Helper: wait until video is ready at current time
        const waitForFrame = (): Promise<void> =>
          new Promise((resolve) => {
            if (video.readyState >= 2) {
              resolve();
            } else {
              video.oncanplay = () => resolve();
            }
          });

        while (!abortRef.current && frame < totalFrames) {
          await waitForFrame();

          // Draw full frame
          ctx.drawImage(video, 0, 0, vw, vh);

          // Apply blur mask over watermark region
          if (mx1 > mx0 && my1 > my0) {
            // Save the region under the mask
            const maskData = ctx.getImageData(mx0, my0, mx1 - mx0, my1 - my0);

            // Create a temporary canvas for blur
            const tmpCanvas = document.createElement("canvas");
            tmpCanvas.width = mx1 - mx0;
            tmpCanvas.height = my1 - my0;
            const tmpCtx = tmpCanvas.getContext("2d")!;
            tmpCtx.putImageData(maskData, 0, 0);

            // Draw blurred version back
            ctx.save();
            ctx.filter = `blur(${mask.blurRadius}px)`;
            ctx.drawImage(tmpCanvas, mx0, my0);
            ctx.restore();

            // Extra smoothing pass for better coverage
            ctx.save();
            ctx.filter = `blur(${Math.max(1, mask.blurRadius / 3)}px)`;
            ctx.globalAlpha = 0.5;
            ctx.drawImage(tmpCanvas, mx0, my0);
            ctx.restore();
            ctx.globalAlpha = 1;
          }

          frame++;
          setProgress({ current: frame, total: totalFrames });

          // Advance video by one frame
          const nextTime = frame / fps;
          if (nextTime < video.duration) {
            video.currentTime = nextTime;
            await new Promise<void>((resolve) => {
              video.onseeked = () => resolve();
            });
          }
        }

        // Stop recorder
        if (recorder.state !== "inactive") {
          recorder.stop();
        }

        if (abortRef.current) {
          URL.revokeObjectURL(videoUrl);
          setIsProcessing(false);
          setProgress(null);
          return;
        }

        const resultBlob = await recorderDone;
        const outUrl = URL.createObjectURL(resultBlob);
        prevUrlRef.current = outUrl;

        const baseName = file.name.replace(/\.[^.]+$/, "");
        setOutputUrl(outUrl);
        setOutputName(`${baseName}-no-watermark.webm`);
        setProgress({ current: totalFrames, total: totalFrames });
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e));
      } finally {
        setIsProcessing(false);
        videoRef.current = null;
      }
    },
    [],
  );

  return { process, progress, outputUrl, outputName, isProcessing, cancel, error };
}
