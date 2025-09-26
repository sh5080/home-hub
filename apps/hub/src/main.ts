import 'reflect-metadata';
import { NestFactory } from '@nestjs/core';
import { FastifyAdapter, NestFastifyApplication } from '@nestjs/platform-fastify';
import { AppModule } from './app.module';
import { loadConfig } from './config';
import { createLogger } from './logger';

async function bootstrap() {
  const config = loadConfig();
  const logger = createLogger(config.server.logLevel);

  logger.info({ bridgeName: config.hap.bridgeName }, 'Starting Home Hub');

  const app = await NestFactory.create<NestFastifyApplication>(
    AppModule,
    new FastifyAdapter(),
    { logger: false },
  );

  await app.listen(config.server.adminPort, '0.0.0.0');
  logger.info({ port: config.server.adminPort }, 'Admin API listening');

  const shutdown = async (signal: string) => {
    logger.info({ signal }, 'Shutting down');
    await app.close();
    process.exit(0);
  };

  process.on('SIGTERM', () => shutdown('SIGTERM'));
  process.on('SIGINT', () => shutdown('SIGINT'));
}

bootstrap();
