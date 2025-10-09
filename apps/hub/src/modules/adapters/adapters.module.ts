import { Module } from '@nestjs/common';
import { AdapterLoaderService } from './adapter-loader.service';

@Module({
  providers: [AdapterLoaderService],
})
export class AdaptersModule {}
