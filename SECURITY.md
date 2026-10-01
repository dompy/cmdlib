# Security

Report suspected vulnerabilities privately through GitHub's "Report a vulnerability" option on
[this repository](https://github.com/dompy/cmdlib/security/advisories/new). Avoid public issues containing
exploit details or private data. Include a synthetic reproducer, version, platform, and expected impact.
There is no promised response time or long-term support policy for this initial release.

cmdlib executes arbitrary shell text only after explicit confirmation. It is not a sandbox or a shell
command analyzer. Risk labels and explanations are user-maintained and may be inaccurate.
A command runs with your user permissions and environment; inspect its full text and prerequisites.
Clipboard operations and Navi exports can expose command text. Stored libraries are not encrypted.

If a credential is disclosed, revoke or rotate it with its issuer. Removing it from a file or Git history
alone does not make the credential safe again.
