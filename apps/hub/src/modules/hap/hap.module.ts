import { Module } from '@nestjs/common';
import { HapService } from './hap.service';

@Module({
  providers: [HapService],
})
export class HapModule {}
