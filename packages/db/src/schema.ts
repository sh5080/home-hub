import { randomUUID } from 'node:crypto';
import { sqliteTable, text, integer } from 'drizzle-orm/sqlite-core';
import { DeviceType, CapabilityKind, EventSource, HubEventType } from '@home-hub/core';

function enumValues<T extends Record<string, string>>(e: T) {
  return Object.values(e) as [T[keyof T], ...T[keyof T][]];
}

export const devices = sqliteTable('devices', {
  id: text('id').primaryKey(),
  name: text('name').notNull(),
  adapterId: text('adapterId').notNull(),
  type: text('type', { enum: enumValues(DeviceType) }).notNull(),
  capabilities: text('capabilities').notNull(), // JSON array of CapabilityKind
  online: integer('online', { mode: 'boolean' }).notNull().default(true),
  createdAt: integer('createdAt', { mode: 'timestamp' })
    .notNull()
    .$defaultFn(() => new Date()),
  updatedAt: integer('updatedAt', { mode: 'timestamp' })
    .notNull()
    .$defaultFn(() => new Date()),
});

export const deviceStates = sqliteTable('deviceStates', {
  id: text('id').primaryKey().$defaultFn(() => randomUUID()),
  deviceId: text('deviceId')
    .notNull()
    .references(() => devices.id),
  capability: text('capability', { enum: enumValues(CapabilityKind) }).notNull(),
  state: text('state').notNull(), // JSON serialized capability state
  source: text('source', { enum: enumValues(EventSource) })
    .notNull()
    .default(EventSource.HUB),
  createdAt: integer('createdAt', { mode: 'timestamp' })
    .notNull()
    .$defaultFn(() => new Date()),
});

export const events = sqliteTable('events', {
  id: text('id').primaryKey().$defaultFn(() => randomUUID()),
  type: text('type', { enum: enumValues(HubEventType) }).notNull(),
  deviceId: text('deviceId'),
  payload: text('payload').notNull(), // JSON
  createdAt: integer('createdAt', { mode: 'timestamp' })
    .notNull()
    .$defaultFn(() => new Date()),
});
