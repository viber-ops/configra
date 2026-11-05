import React, { useEffect, useId, useRef, useState } from 'react';
import { localDateTime } from '../token-validation.js';

const day = 86400000;

export function useCurrentTime() {
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    const update = () => setNow(Date.now());
    const timer = setInterval(update, 60000);
    document.addEventListener('visibilitychange', update);
    return () => { clearInterval(timer); document.removeEventListener('visibilitychange', update); };
  }, []);
  return now;
}

function duration(milliseconds, zh) {
  // Round only the display to a minute. Limit checks use the unrounded duration.
  const minutes = Math.max(1, Math.ceil(milliseconds / 60000));
  return [[Math.floor(minutes / 1440), 'day', '天'], [Math.floor(minutes % 1440 / 60), 'hour', '小时'], [minutes % 60, 'minute', '分钟']]
    .filter(([count]) => count > 0).map(([count, en, cn]) => zh ? `${count} ${cn}` : `${count} ${en}${count === 1 ? '' : 's'}`).join(' ');
}

function dateTime(value, locale) {
  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value));
}

export function Expiration({ value, now, t, revoked = false }) {
  if (!value) return <span>{t.neverExpires}</span>;
  const timestamp = Date.parse(value);
  if (!Number.isFinite(timestamp)) return <span>—</span>;
  const zh = t.locale.startsWith('zh');
  const remaining = timestamp - now;
  return <span className="credential-expiration"><time dateTime={value} title={value}>{dateTime(timestamp, t.locale)}</time>
    {!revoked && <small className={remaining <= 0 ? 'expired-hint' : ''}>{remaining <= 0 ? (zh ? '已过期' : 'Expired') : `${zh ? '剩余' : 'Remaining:'} ${duration(remaining, zh)}`}</small>}
  </span>;
}

export function ExpiryInput({ t, maxDays, feedback, onEdit }) {
  const now = Date.now();
  const id = useId();
  const input = useRef(null);
  const [value, setValue] = useState('');
  const [incomplete, setIncomplete] = useState(false);
  const zh = t.locale.startsWith('zh');
  const remaining = value ? Date.parse(value) - now : NaN;
  const tooLong = maxDays != null && remaining > maxDays * day;
  const invalid = incomplete || (value !== '' && (!Number.isFinite(remaining) || remaining <= 0 || tooLong));
  const deadline = maxDays == null ? undefined : localDateTime(new Date(now + maxDays * day));
  const edited = event => { setValue(event.target.value); setIncomplete(event.target.validity.badInput); onEdit(); };
  const choose = days => {
    const selected = localDateTime(new Date(Date.now() + days * day));
    input.current.value = selected;
    setValue(selected); setIncomplete(false); onEdit();
  };
  const presets = [7, 30].filter(days => maxDays == null || days < maxDays);
  let summary = '';
  if (incomplete) summary = zh ? '请补全日期和时间以计算有效期。' : 'Complete the date and time to calculate the lifetime.';
  else if (Number.isFinite(remaining)) summary = remaining <= 0 ? (zh ? '所选时间已过去，请选择未来时间。' : 'This time has passed. Choose a future expiry.') : `${zh ? '所选有效期：' : 'Selected lifetime: '}${duration(remaining, zh)}`;
  return <div className="expiry-field">
    <label htmlFor={id}>{t.expiresAt}</label>
    <input ref={input} id={id} name="expires_at" type="datetime-local" required={maxDays != null} max={deadline} aria-label={t.expiresAt} onInput={edited}
      aria-invalid={feedback?.['aria-invalid'] || invalid || undefined} aria-describedby={[`${id}-hint`, summary && `${id}-summary`, feedback?.['aria-describedby']].filter(Boolean).join(' ')} />
    <div className="expiry-presets" role="group" aria-label={zh ? '选择有效天数' : 'Choose lifetime'}>
      {presets.map(days => <button key={days} type="button" onClick={() => choose(days)}>{zh ? `${days} 天` : `${days} days`}</button>)}
      {maxDays != null && <button type="button" onClick={() => choose(maxDays)}>{zh ? `使用最长有效期（${maxDays} 天）` : `Use maximum (${maxDays} days)`}</button>}
    </div>
    {summary && <div id={`${id}-summary`} role="status" className={`expiry-summary ${invalid ? 'expiry-invalid' : ''}`}><span>{summary}</span>{tooLong && <span>{zh ? `超过 ${maxDays} 天上限 ${duration(remaining - maxDays * day, zh)}。` : `Exceeds the ${maxDays}-day limit by ${duration(remaining - maxDays * day, zh)}.`}</span>}</div>}
    <small id={`${id}-hint`}>{maxDays != null ? t.writeTokenDeadline.replace('{days}', maxDays).replace('{date}', deadline.replace('T', ' ')) : (zh ? '留空使用默认的 90 天有效期。' : 'Leave empty for the default 90-day lifetime.')}</small>
  </div>;
}

export function ValidityPreview({ days, now, authority, t }) {
  const count = Number(days);
  if (!Number.isInteger(count) || count < 1 || count > (authority ? 365 : 3650)) return null;
  const requested = now + count * day;
  const limit = authority ? Date.parse(authority.not_after) : Infinity;
  const effective = Math.min(requested, limit);
  if (!Number.isFinite(effective)) return null;
  const zh = t.locale.startsWith('zh');
  return <div className="validity-preview" role="status">
    <span>{zh ? '预计到期：' : 'Estimated expiry: '}{dateTime(effective, t.locale)}</span>
    {effective <= now ? <span className="expired-hint">{zh ? 'CA 已过期，请选择其他 CA。' : 'The CA has expired. Choose another CA.'}</span> : <span>{zh ? '有效期：' : 'Lifetime: '}{duration(effective - now, zh)}{limit < requested && <strong> · {zh ? '受 CA 到期时间限制' : 'Limited by CA expiry'}</strong>}</span>}
  </div>;
}
