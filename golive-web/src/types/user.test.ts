import { describe, expect, it } from 'vitest';

import {
  creatorName,
  isPlaceholderName,
  isUuidLike,
  personName,
  userDisplayName,
  userName,
} from './user';

const uuid = '550e8400-e29b-41d4-a716-446655440000';

describe('user helpers', () => {
  it('recognizes UUID-like usernames with surrounding whitespace', () => {
    expect(isUuidLike(` ${uuid} `)).toBe(true);
    expect(isUuidLike('streamer')).toBe(false);
    expect(isUuidLike(null)).toBe(false);
  });

  it('chooses the most human-readable name, never one made from the id', () => {
    expect(userName({ username: 'streamer', displayName: ' Streamer ' })).toBe('Streamer');
    expect(userName({ username: 'streamer', displayName: '  ' })).toBe('streamer');
    expect(userName({ username: uuid, displayName: '' })).toBe('');
    expect(userName(null)).toBe('');

    expect(userDisplayName(null)).toBe('You');
    expect(userDisplayName({ id: 'u1', username: 'streamer', displayName: 'Streamer' })).toBe(
      'Streamer',
    );
    expect(userDisplayName({ id: uuid, username: uuid, displayName: '' })).toBe('Unknown user');
  });

  it.each([
    ['', true],
    ['   ', true],
    [uuid, true],
    ['creator', false],
    ['Creator abcdef', true],
    ['Creator 550e8400', true],
    ['Unknown user', true],
    ['Unknown creator', true],
    ['Creator Studio', false],
    ['Creator Fans', false],
    ['GoLive Studio', false],
  ])('detects whether %j is a placeholder name', (name, expected) => {
    expect(isPlaceholderName(name)).toBe(expected);
  });

  it('labels server names that are placeholders', () => {
    expect(personName(' Luna ')).toBe('Luna');
    expect(personName('')).toBe('Unknown user');
    expect(personName(uuid)).toBe('Unknown user');
    expect(creatorName('Creator 550e8400')).toBe('Unknown creator');
    expect(creatorName(undefined)).toBe('Unknown creator');
  });
});
