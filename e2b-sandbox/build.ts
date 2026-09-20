import 'dotenv/config';
import { Template, defaultBuildLogger } from 'e2b';
import { scandrixTemplate } from './template';

async function main() {
    const template = await Template.build(scandrixTemplate, {
        alias: 'scandrix-sandbox',
        cpuCount: 2,
        memoryMB: 1024,
        onBuildLogs: defaultBuildLogger(),
    });

    console.log(`\n✅ ScanDrix template ready!\nID: ${template.templateID}\nAdd to .env: E2B_TEMPLATE_ID=${template.templateID}`);
}

main().catch(console.error);
