/**
 * Commit Message Format:
 * [project] type(scope): subject
 *
 * Examples:
 * [api] feat(auth): add jwt authentication
 * [worker] fix(consumer): ack only after the notification is stored
 * [shared] refactor(utils): optimize date formatting
 * [worker] perf(queue): improve message processing
 * [tools] feat(codegen): generate collection constants
 * [api] feat(user): add user's profile page
 * [ci] fix(lint): pin golangci-lint version
 * 
 * Types:
 * feat     - New features or significant changes
 * fix      - Bug fixes or error corrections
 * docs     - Documentation changes only
 * style    - Code style changes (formatting, missing semicolons, etc)
 * refactor - Code changes that neither fixes a bug nor adds a feature
 * perf     - Performance improvements
 * test     - Adding or updating tests
 * build    - Changes to build system or external dependencies
 * ci       - Changes to CI configuration and scripts
 * chore    - Other changes that don't modify src or test files
 * revert   - Reverts a previous commit
 *
 * Notes:
 * - Project must be one of: api, worker, shared, ci, docs, deploy, tools (lowercase only)
 * - Scope should be lowercase and describe the module or component
 * - Subject must be in English and use imperative mood
 * - Subject should only contain letters, numbers, spaces, and allowed punctuations (, - ' ! . / # _ { })
 * - Max header length is 100 characters
 *
 * Common mistakes to avoid:
 * ❌ [API] feat(auth): add login                  (project should be lowercase)
 * ❌ [api] fix(Editor): fix upload               (scope should be lowercase)
 * ❌ [web] feat(user): add Login                 ("web" is not a valid project)
 * ❌ [api] Feat(auth): implement auth            (type should be lowercase)
 * ❌ [api] feat(auth):implement login            (missing space after colon)
 * ✅ [api] feat(auth): implement user login
 * ✅ [api] feat(user): add user's profile
 * ✅ [api] fix(auth): reject refresh tokens on access routes
 */

module.exports = {
  extends: ['@commitlint/config-conventional'],
  parserPreset: {
    parserOpts: {
      headerPattern: /^\[([a-z0-9-]+)\]\s+([a-z]+)\(([a-z0-9-]+)\):\s+(.+)$/,
      headerCorrespondence: ['project', 'type', 'scope', 'subject'],
    },
  },
  rules: {
    // Validate project name
    'project-enum': [2, 'always'],
    // Enforce max header length
    'header-max-length': [2, 'always', 100],
    // Define allowed commit types
    'type-enum': [
      2,
      'always',
      ['feat', 'fix', 'docs', 'style', 'refactor', 'perf', 'test', 'build', 'ci', 'chore', 'revert'],
    ],
    // Require non-empty subject
    'subject-empty': [2, 'never'],
    // Prevent period at end of subject
    'subject-full-stop': [2, 'never', '.'],
    // Require scope
    'scope-empty': [2, 'never'],
    // Enforce subject format (letters, numbers, spaces, and allowed punctuations)
    'subject-format': [2, 'always'],
  },
  plugins: [
    {
      rules: {
        'project-enum': ({ project }) => {
          const projects = ['api', 'worker', 'shared', 'ci', 'docs', 'deploy', 'tools'];
          return [
            projects.includes(project),
            `project must be one of [${projects.join(', ')}]`,
          ];
        },
        'subject-format': ({ subject }) => {
          // Only allow:
          // - Letters (a-z, A-Z)
          // - Numbers (0-9)
          // - Spaces
          // - Punctuations: , - ' ! . / # _ { }
          const formatRegex = /^[a-zA-Z0-9\s,\-'!./#_{}]+$/;
          return [
            formatRegex.test(subject),
            "Subject may include letters, digits, spaces, and these punctuations: , - ' ! . / # _ { }",
          ];
        },
      },
    },
  ],
};