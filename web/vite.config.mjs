import { defineConfig } from 'vite';
import licenseMaterials from './scripts/license-plugin.mjs';

export default defineConfig({ base: '/ui/', plugins: [licenseMaterials()] });
