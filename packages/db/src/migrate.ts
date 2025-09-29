import { migrate } from 'drizzle-orm/better-sqlite3/migrator';
import { createDb } from './client';

import * as path from 'node:path';
const projectRoot = path.resolve(__dirname, '..', '..', '..');
const dbPath = process.argv[2] || path.join(projectRoot, '.data/hub.db');
const db = createDb(dbPath);
migrate(db, { migrationsFolder: './drizzle' });
console.log('Migrations applied successfully');
