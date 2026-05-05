import { useEffect, useRef, useState } from 'react';
import { useIsFetching } from '@tanstack/react-query';
import { useLocation } from 'react-router-dom';
import { cn } from '@/lib/cn';

const MIN_VISIBLE_MS = 340;
const COMPLETE_HIDE_MS = 180;

export function TopProgressBar() {
  const location = useLocation();
  const fetchingCount = useIsFetching();
  const [visible, setVisible] = useState(false);
  const [settling, setSettling] = useState(false);
  const [progress, setProgress] = useState(0);
  const activeRef = useRef(false);
  const fetchingRef = useRef(fetchingCount);
  const lastLocationKeyRef = useRef(location.key);
  const startedAtRef = useRef(0);
  const timersRef = useRef<number[]>([]);
  const trickleRef = useRef<number | null>(null);

  const clearTimers = () => {
    timersRef.current.forEach((timer) => window.clearTimeout(timer));
    timersRef.current = [];
    if (trickleRef.current !== null) {
      window.clearInterval(trickleRef.current);
      trickleRef.current = null;
    }
  };

  const schedule = (fn: () => void, delay: number) => {
    const timer = window.setTimeout(fn, delay);
    timersRef.current.push(timer);
  };

  const begin = () => {
    clearTimers();
    activeRef.current = true;
    startedAtRef.current = performance.now();
    setSettling(false);
    setVisible(true);
    setProgress(0.08);
    schedule(() => setProgress((current) => Math.max(current, 0.32)), 80);
    schedule(() => setProgress((current) => Math.max(current, 0.52)), 190);
  };

  const finish = () => {
    if (!activeRef.current) return;
    if (trickleRef.current !== null) {
      window.clearInterval(trickleRef.current);
      trickleRef.current = null;
    }
    const elapsed = performance.now() - startedAtRef.current;
    const wait = Math.max(0, MIN_VISIBLE_MS - elapsed);
    schedule(() => {
      if (fetchingRef.current > 0) return;
      setSettling(true);
      setProgress(1);
      schedule(() => {
        activeRef.current = false;
        setVisible(false);
        setSettling(false);
        setProgress(0);
      }, COMPLETE_HIDE_MS);
    }, wait);
  };

  const ensureTrickle = () => {
    if (!activeRef.current || trickleRef.current !== null) return;
    trickleRef.current = window.setInterval(() => {
      setProgress((current) => {
        const ceiling = fetchingRef.current > 0 ? 0.86 : 0.94;
        const step = current < 0.45 ? 0.08 : current < 0.7 ? 0.035 : 0.014;
        return Math.min(ceiling, current + step);
      });
    }, 220);
  };

  useEffect(() => {
    if (lastLocationKeyRef.current === location.key) {
      return;
    }
    lastLocationKeyRef.current = location.key;
    begin();
    if (fetchingRef.current > 0) {
      ensureTrickle();
    } else {
      finish();
    }
    return clearTimers;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [location.key]);

  useEffect(() => {
    fetchingRef.current = fetchingCount;
    if (!activeRef.current) {
      return;
    }
    if (fetchingCount > 0) {
      ensureTrickle();
      return;
    }
    finish();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fetchingCount]);

  useEffect(() => clearTimers, []);

  return (
    <div
      className={cn('gl-top-progress', visible && 'is-visible', settling && 'is-settling')}
      aria-hidden="true"
    >
      <span style={{ transform: `scaleX(${progress})` }} />
    </div>
  );
}
