#!/usr/bin/env tsx
/**
 * ================================================================
 * Smartsheet Project Management CLI
 * ================================================================
 * A comprehensive CLI tool for engineers to manage projects in Smartsheet
 *
 * Features:
 * - Create and manage sheets
 * - Add tasks/rows to existing sheets
 * - List projects, sheets, and workspaces
 * - Update task status and properties
 * - Project templates for engineering workflows
 *
 * Requirements:
 * - SMARTSHEETS_API_KEY environment variable
 * - Node.js 14+ with TypeScript
 *
 * Usage:
 *   smartsheet-cli list sheets
 *   smartsheet-cli create sheet "My Project"
 *   smartsheet-cli add task <sheet-id> "Task name"
 *   smartsheet-cli create project "Project Name"
 * ================================================================
 */

import { createClient } from 'smartsheet';
import { Command } from 'commander';
import chalk from 'chalk';
import ora from 'ora';
import prompts from 'prompts';
import Table from 'cli-table3';

// ================================================================
// Types and Interfaces
// ================================================================

interface SmartsheetConfig {
  accessToken: string;
  logLevel?: 'info' | 'debug' | 'error';
}

interface SheetColumn {
  title: string;
  type: 'TEXT_NUMBER' | 'PICKLIST' | 'DATE' | 'CHECKBOX' | 'CONTACT_LIST';
  options?: string[];
  primary?: boolean;
  width?: number;
}

interface TaskRow {
  taskName: string;
  status?: string;
  assignee?: string;
  dueDate?: string;
  priority?: string;
  description?: string;
}

interface ProjectTemplate {
  name: string;
  description: string;
  columns: SheetColumn[];
}

// ================================================================
// Constants
// ================================================================

const STATUS_OPTIONS = ['Not Started', 'In Progress', 'Completed', 'Blocked', 'On Hold'];
const PRIORITY_OPTIONS = ['Low', 'Medium', 'High', 'Critical'];

// Project templates for engineering workflows
const PROJECT_TEMPLATES: Record<string, ProjectTemplate> = {
  standard: {
    name: 'Standard Engineering Project',
    description: 'Basic task tracking for engineering projects',
    columns: [
      { title: 'Task Name', type: 'TEXT_NUMBER', primary: true, width: 300 },
      { title: 'Status', type: 'PICKLIST', options: STATUS_OPTIONS, width: 120 },
      { title: 'Assignee', type: 'CONTACT_LIST', width: 150 },
      { title: 'Priority', type: 'PICKLIST', options: PRIORITY_OPTIONS, width: 100 },
      { title: 'Due Date', type: 'DATE', width: 120 },
      { title: 'Est. Hours', type: 'TEXT_NUMBER', width: 100 },
      { title: 'Description', type: 'TEXT_NUMBER', width: 300 },
    ],
  },
  sprint: {
    name: 'Sprint Planning',
    description: 'Agile sprint planning and tracking',
    columns: [
      { title: 'Story/Task', type: 'TEXT_NUMBER', primary: true, width: 300 },
      { title: 'Type', type: 'PICKLIST', options: ['Story', 'Task', 'Bug', 'Tech Debt'], width: 100 },
      { title: 'Status', type: 'PICKLIST', options: STATUS_OPTIONS, width: 120 },
      { title: 'Assignee', type: 'CONTACT_LIST', width: 150 },
      { title: 'Story Points', type: 'TEXT_NUMBER', width: 100 },
      { title: 'Sprint', type: 'TEXT_NUMBER', width: 100 },
      { title: 'Priority', type: 'PICKLIST', options: PRIORITY_OPTIONS, width: 100 },
    ],
  },
  bugs: {
    name: 'Bug Tracking',
    description: 'Track and manage software bugs',
    columns: [
      { title: 'Bug Title', type: 'TEXT_NUMBER', primary: true, width: 300 },
      { title: 'Severity', type: 'PICKLIST', options: ['Critical', 'High', 'Medium', 'Low'], width: 100 },
      {
        title: 'Status',
        type: 'PICKLIST',
        options: ['Open', 'In Progress', 'Fixed', 'Verified', 'Closed'],
        width: 120,
      },
      { title: 'Assignee', type: 'CONTACT_LIST', width: 150 },
      { title: 'Found In', type: 'TEXT_NUMBER', width: 100 },
      { title: 'Fixed In', type: 'TEXT_NUMBER', width: 100 },
      { title: 'Reporter', type: 'CONTACT_LIST', width: 150 },
      { title: 'Description', type: 'TEXT_NUMBER', width: 300 },
    ],
  },
  releases: {
    name: 'Release Planning',
    description: 'Track release milestones and deliverables',
    columns: [
      { title: 'Feature/Task', type: 'TEXT_NUMBER', primary: true, width: 300 },
      { title: 'Release', type: 'TEXT_NUMBER', width: 100 },
      { title: 'Status', type: 'PICKLIST', options: STATUS_OPTIONS, width: 120 },
      { title: 'Owner', type: 'CONTACT_LIST', width: 150 },
      { title: 'Target Date', type: 'DATE', width: 120 },
      { title: 'Dependencies', type: 'TEXT_NUMBER', width: 200 },
      { title: 'Notes', type: 'TEXT_NUMBER', width: 300 },
    ],
  },
};

// ================================================================
// Smartsheet Client Setup
// ================================================================

function initializeClient(): any {
  const accessToken = process.env.SMARTSHEETS_API_KEY;

  if (!accessToken) {
    console.error(chalk.red('Error: SMARTSHEETS_API_KEY environment variable not set'));
    console.log(chalk.yellow('\nPlease set your API key:'));
    console.log(chalk.cyan('  export SMARTSHEETS_API_KEY="your-api-key-here"'));
    console.log(chalk.yellow('\nOr add it to your .env file'));
    process.exit(1);
  }

  const config: SmartsheetConfig = {
    accessToken,
    logLevel: process.env.DEBUG ? 'debug' : 'error',
  };

  return createClient(config);
}

// ================================================================
// Utility Functions
// ================================================================

function formatDate(date?: string): string {
  if (!date) return '-';
  return new Date(date).toLocaleDateString();
}

function truncate(str: string, maxLen: number): string {
  if (str.length <= maxLen) return str;
  return str.substring(0, maxLen - 3) + '...';
}

function findColumnByTitle(sheet: any, title: string): any {
  return sheet.columns.find((col: any) => col.title.toLowerCase() === title.toLowerCase());
}

function getCellValue(row: any, columnId: number): any {
  const cell = row.cells.find((c: any) => c.columnId === columnId);
  return cell?.value || cell?.displayValue || '-';
}

// ================================================================
// Command: List Sheets
// ================================================================

async function listSheets(options: any): Promise<void> {
  const spinner = ora('Fetching sheets...').start();
  const client = initializeClient();

  try {
    const response = await client.sheets.listSheets({
      includeAll: true,
    });

    spinner.succeed(`Found ${response.data.length} sheets`);

    if (response.data.length === 0) {
      console.log(chalk.yellow('\nNo sheets found. Create one with:'));
      console.log(chalk.cyan('  smartsheet-cli create sheet "Sheet Name"'));
      return;
    }

    const table = new Table({
      head: ['ID', 'Name', 'Modified', 'Access Level'].map((h) => chalk.cyan(h)),
      colWidths: [20, 40, 20, 15],
    });

    response.data
      .sort((a: any, b: any) => new Date(b.modifiedAt).getTime() - new Date(a.modifiedAt).getTime())
      .forEach((sheet: any) => {
        table.push([sheet.id, truncate(sheet.name, 38), formatDate(sheet.modifiedAt), sheet.accessLevel || '-']);
      });

    console.log('\n' + table.toString());

    if (options.verbose) {
      console.log(chalk.gray(`\nTotal: ${response.data.length} sheets`));
    }
  } catch (error: any) {
    spinner.fail('Failed to fetch sheets');
    console.error(chalk.red(`Error: ${error.message}`));
    process.exit(1);
  }
}

// ================================================================
// Command: List Workspaces
// ================================================================

async function listWorkspaces(): Promise<void> {
  const spinner = ora('Fetching workspaces...').start();
  const client = initializeClient();

  try {
    const response = await client.workspaces.listWorkspaces({
      includeAll: true,
    });

    spinner.succeed(`Found ${response.data.length} workspaces`);

    if (response.data.length === 0) {
      console.log(chalk.yellow('\nNo workspaces found.'));
      return;
    }

    const table = new Table({
      head: ['ID', 'Name', 'Access Level'].map((h) => chalk.cyan(h)),
      colWidths: [20, 50, 15],
    });

    response.data.forEach((workspace: any) => {
      table.push([workspace.id, truncate(workspace.name, 48), workspace.accessLevel || '-']);
    });

    console.log('\n' + table.toString());
  } catch (error: any) {
    spinner.fail('Failed to fetch workspaces');
    console.error(chalk.red(`Error: ${error.message}`));
    process.exit(1);
  }
}

// ================================================================
// Command: Get Sheet Details
// ================================================================

async function getSheetDetails(sheetId: string): Promise<void> {
  const spinner = ora('Fetching sheet details...').start();
  const client = initializeClient();

  try {
    const response = await client.sheets.getSheet({ id: parseInt(sheetId) });

    // Debug: Log the response structure if DEBUG is set
    if (process.env.DEBUG) {
      console.log('\n[DEBUG] Response structure:', JSON.stringify(response, null, 2));
    }

    // The SDK returns different structures depending on the call
    // For getSheet with pagination metadata, data contains the sheet
    // For direct calls, the sheet is in the response itself
    let sheet;

    // Check if response has pagination metadata with data array
    if (response.data !== undefined) {
      if (Array.isArray(response.data) && response.data.length > 0) {
        sheet = response.data[0];
      } else if (typeof response.data === 'object' && response.data !== null) {
        sheet = response.data;
      }
    }

    // Fallback: sheet data might be directly in response
    if (!sheet && response.name) {
      sheet = response;
    }

    // If still no sheet, provide helpful error with actual response
    if (!sheet || !sheet.name) {
      if (process.env.DEBUG) {
        console.error('\n[DEBUG] Full response:', JSON.stringify(response, null, 2));
      }
      throw new Error(
        'Unable to extract sheet data from response. ' +
          'Response has keys: ' +
          Object.keys(response).join(', ') +
          (response.data ? ' | data type: ' + (Array.isArray(response.data) ? 'array' : typeof response.data) : '') +
          '. Try with DEBUG=1 for full response.'
      );
    }

    spinner.succeed(`Sheet: ${sheet.name}`);

    console.log(chalk.cyan('\n=== Sheet Information ==='));
    console.log(`Name: ${chalk.white(sheet.name)}`);
    console.log(`ID: ${chalk.white(sheet.id)}`);
    console.log(`Rows: ${chalk.white(sheet.rows?.length || 0)}`);
    console.log(`Columns: ${chalk.white(sheet.columns?.length || 0)}`);
    console.log(`Modified: ${chalk.white(formatDate(sheet.modifiedAt))}`);

    // Show columns
    if (sheet.columns && sheet.columns.length > 0) {
      console.log(chalk.cyan('\n=== Columns ==='));
      const colTable = new Table({
        head: ['Title', 'Type', 'Primary'].map((h) => chalk.cyan(h)),
      });

      sheet.columns.forEach((col: any) => {
        colTable.push([col.title, col.type, col.primary ? '✓' : '']);
      });

      console.log(colTable.toString());
    }

    // Show recent rows (limit to 10)
    if (sheet.rows && sheet.rows.length > 0) {
      console.log(chalk.cyan('\n=== Recent Tasks (showing first 10) ==='));
      const primaryCol = sheet.columns.find((c: any) => c.primary);

      const taskTable = new Table({
        head: ['Row ID', 'Task Name'].map((h) => chalk.cyan(h)),
        colWidths: [10, 70],
      });

      sheet.rows.slice(0, 10).forEach((row: any) => {
        const primaryValue = primaryCol ? getCellValue(row, primaryCol.id) : row.cells[0]?.value || '-';

        taskTable.push([row.id || row.rowNumber, truncate(String(primaryValue), 68)]);
      });

      console.log(taskTable.toString());

      if (sheet.rows.length > 10) {
        console.log(chalk.gray(`... and ${sheet.rows.length - 10} more rows`));
      }
    }
  } catch (error: any) {
    spinner.fail('Failed to fetch sheet details');
    console.error(chalk.red(`Error: ${error.message}`));
    if (error.stack) {
      console.error(chalk.gray('\nDebug info:'));
      console.error(chalk.gray(error.stack));
    }
    process.exit(1);
  }
}

// ================================================================
// Command: Create Sheet
// ================================================================

async function createSheet(name: string, options: any): Promise<void> {
  const client = initializeClient();

  // Select template
  let template: ProjectTemplate;

  if (options.template && PROJECT_TEMPLATES[options.template]) {
    template = PROJECT_TEMPLATES[options.template];
  } else if (options.interactive) {
    const promptResponse = await prompts({
      type: 'select',
      name: 'template',
      message: 'Select a project template:',
      choices: Object.entries(PROJECT_TEMPLATES).map(([key, tmpl]) => ({
        title: tmpl.name,
        description: tmpl.description,
        value: key,
      })),
    });

    if (!promptResponse.template) {
      console.log(chalk.yellow('Cancelled'));
      return;
    }

    template = PROJECT_TEMPLATES[promptResponse.template];
  } else {
    template = PROJECT_TEMPLATES.standard;
  }

  const spinner = ora(`Creating sheet "${name}" with ${template.name} template...`).start();

  try {
    const sheetSpec = {
      name,
      columns: template.columns.map((col, index) => ({
        title: col.title,
        type: col.type,
        primary: col.primary || false,
        index,
        width: col.width,
        ...(col.options && { options: col.options }),
      })),
    };

    const response = await client.sheets.createSheet({ body: sheetSpec });

    const result = response.data || response.result || response;

    spinner.succeed(`Sheet created successfully!`);
    console.log(chalk.green(`\nSheet ID: ${result.id}`));
    console.log(chalk.cyan(`Sheet URL: ${result.permalink || 'N/A'}`));
    console.log(chalk.gray(`\nTemplate: ${template.name}`));
    console.log(chalk.gray(`Columns: ${template.columns.length}`));
  } catch (error: any) {
    spinner.fail('Failed to create sheet');
    console.error(chalk.red(`Error: ${error.message}`));
    if (error.response?.data) {
      console.error(chalk.gray(JSON.stringify(error.response.data, null, 2)));
    }
    process.exit(1);
  }
}

// ================================================================
// Command: Add Task
// ================================================================

async function addTask(sheetId: string, taskName: string, options: any): Promise<void> {
  const client = initializeClient();
  const spinner = ora('Adding task...').start();

  try {
    // Get sheet to find column IDs
    spinner.text = 'Fetching sheet structure...';
    const sheetResponse = await client.sheets.getSheet({ id: parseInt(sheetId) });
    const sheet = sheetResponse.data || sheetResponse;

    // Find columns
    const primaryCol = sheet.columns.find((c: any) => c.primary);
    const statusCol = findColumnByTitle(sheet, 'Status');
    const assigneeCol = findColumnByTitle(sheet, 'Assignee');
    const priorityCol = findColumnByTitle(sheet, 'Priority');
    const dueDateCol = findColumnByTitle(sheet, 'Due Date');

    if (!primaryCol) {
      throw new Error('Sheet has no primary column');
    }

    // Build row cells
    const cells: any[] = [{ columnId: primaryCol.id, value: taskName }];

    if (options.status && statusCol) {
      cells.push({ columnId: statusCol.id, value: options.status });
    }

    if (options.assignee && assigneeCol) {
      cells.push({ columnId: assigneeCol.id, value: options.assignee });
    }

    if (options.priority && priorityCol) {
      cells.push({ columnId: priorityCol.id, value: options.priority });
    }

    if (options.dueDate && dueDateCol) {
      cells.push({ columnId: dueDateCol.id, value: options.dueDate });
    }

    // Add row
    spinner.text = 'Adding task to sheet...';
    const rowSpec = {
      toBottom: true,
      cells,
    };

    const addRowResponse = await client.sheets.addRows({
      sheetId: parseInt(sheetId),
      body: [rowSpec],
    });

    const result = addRowResponse.data || addRowResponse.result || addRowResponse;
    const addedRow = Array.isArray(result) ? result[0] : result[0];

    spinner.succeed('Task added successfully!');
    console.log(chalk.green(`\nTask: ${taskName}`));
    console.log(chalk.gray(`Row ID: ${addedRow?.id || 'N/A'}`));

    if (options.status) console.log(chalk.gray(`Status: ${options.status}`));
    if (options.assignee) console.log(chalk.gray(`Assignee: ${options.assignee}`));
    if (options.priority) console.log(chalk.gray(`Priority: ${options.priority}`));
    if (options.dueDate) console.log(chalk.gray(`Due Date: ${options.dueDate}`));
  } catch (error: any) {
    spinner.fail('Failed to add task');
    console.error(chalk.red(`Error: ${error.message}`));
    process.exit(1);
  }
}

// ================================================================
// Command: Update Task Status
// ================================================================

async function updateTaskStatus(sheetId: string, rowId: string, status: string): Promise<void> {
  const client = initializeClient();
  const spinner = ora('Updating task status...').start();

  try {
    // Get sheet to find status column
    spinner.text = 'Fetching sheet structure...';
    const sheetResponse = await client.sheets.getSheet({ id: parseInt(sheetId) });
    const sheet = sheetResponse.data || sheetResponse;

    const statusCol = findColumnByTitle(sheet, 'Status');

    if (!statusCol) {
      throw new Error('Sheet has no Status column');
    }

    // Update row
    spinner.text = 'Updating status...';
    const rowSpec = {
      id: parseInt(rowId),
      cells: [{ columnId: statusCol.id, value: status }],
    };

    await client.sheets.updateRows({
      sheetId: parseInt(sheetId),
      body: [rowSpec],
    });

    spinner.succeed(`Task status updated to: ${chalk.green(status)}`);
  } catch (error: any) {
    spinner.fail('Failed to update task status');
    console.error(chalk.red(`Error: ${error.message}`));
    process.exit(1);
  }
}

// ================================================================
// Command: List Templates
// ================================================================

function listTemplates(): void {
  console.log(chalk.cyan('\n=== Available Project Templates ===\n'));

  Object.entries(PROJECT_TEMPLATES).forEach(([key, template]) => {
    console.log(chalk.green(`${key}:`));
    console.log(`  ${template.description}`);
    console.log(chalk.gray(`  Columns: ${template.columns.map((c) => c.title).join(', ')}`));
    console.log();
  });

  console.log(chalk.yellow('Usage:'));
  console.log(chalk.cyan('  smartsheet-cli create sheet "My Project" -t standard'));
  console.log(chalk.cyan('  smartsheet-cli create sheet "Sprint 1" -t sprint'));
}

// ================================================================
// Main CLI Setup
// ================================================================

const program = new Command();

program
  .name('smartsheet-cli')
  .description('CLI tool for managing Smartsheet projects (engineer-focused)')
  .version('1.0.0');

// List commands
const listCmd = program.command('list').description('List Smartsheet resources');

listCmd
  .command('sheets')
  .description('List all sheets')
  .option('-v, --verbose', 'Show detailed information')
  .action(listSheets);

listCmd.command('workspaces').description('List all workspaces').action(listWorkspaces);

listCmd.command('templates').description('List available project templates').action(listTemplates);

// Get commands
const getCmd = program.command('get').description('Get details about specific resources');

getCmd.command('sheet <sheet-id>').description('Get sheet details and preview tasks').action(getSheetDetails);

// Create commands
const createCmd = program.command('create').description('Create new Smartsheet resources');

createCmd
  .command('sheet <name>')
  .description('Create a new sheet')
  .option('-t, --template <type>', 'Template type (standard, sprint, bugs, releases)')
  .option('-i, --interactive', 'Interactive template selection')
  .action(createSheet);

// Add commands
const addCmd = program.command('add').description('Add items to existing sheets');

addCmd
  .command('task <sheet-id> <task-name>')
  .description('Add a task to a sheet')
  .option('-s, --status <status>', 'Task status')
  .option('-a, --assignee <email>', 'Assignee email')
  .option('-p, --priority <priority>', 'Task priority')
  .option('-d, --due-date <date>', 'Due date (YYYY-MM-DD)')
  .action(addTask);

// Update commands
const updateCmd = program.command('update').description('Update existing items');

updateCmd.command('status <sheet-id> <row-id> <status>').description('Update task status').action(updateTaskStatus);

// Parse command line arguments
program.parse();
