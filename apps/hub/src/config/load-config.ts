import * as fs from 'node:fs';
import * as path from 'node:path';
import * as yaml from 'js-yaml';
import { hubConfigSchema, type HubConfig } from './hub-config.schema';

export function getProjectRoot(): string {
  // Walk up from apps/hub/src/config → home-hub root
  return path.resolve(__dirname, '..', '..', '..', '..');
}

export function loadConfig(configPath?: string): HubConfig {
  const projectRoot = getProjectRoot();
  const resolved = configPath ?? path.resolve(projectRoot, 'config/hub.yaml');

  if (!fs.existsSync(resolved)) {
    throw new Error(
      `Config file not found: ${resolved}\nCopy config/hub.example.yaml to config/hub.yaml and edit it.`,
    );
  }

  const raw = fs.readFileSync(resolved, 'utf-8');
  const parsed = yaml.load(raw);
  return hubConfigSchema.parse(parsed);
}
