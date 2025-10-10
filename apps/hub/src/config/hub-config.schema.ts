import { z } from 'zod';
import { LogLevel } from '@home-hub/core';

const adapterRefSchema = z.object({
  id: z.string(),
  configFile: z.string(),
});

export const hubConfigSchema = z.object({
  server: z.object({
    adminPort: z.number().default(8080),
    logLevel: z.nativeEnum(LogLevel).default(LogLevel.INFO),
  }),
  hap: z.object({
    bridgeName: z.string().default('Home Hub'),
    pin: z.string().regex(/^\d{3}-\d{2}-\d{3}$/, 'HAP PIN must be XXX-XX-XXX format'),
    setupId: z.string().length(4).default('HUB1'),
    port: z.number().default(51826),
    storagePath: z.string().default('./.data/hap'),
  }),
  database: z.object({
    path: z.string().default('./.data/hub.db'),
  }),
  adapters: z.array(adapterRefSchema).default([]),
});

export type HubConfig = z.infer<typeof hubConfigSchema>;
