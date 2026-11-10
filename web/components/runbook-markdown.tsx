"use client";

import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

/**
 * Renders a runbook's markdown for people to read (styles: .runbook-prose in
 * globals.css). Raw HTML in the text is shown as text, never rendered, and
 * links open outside the module frame.
 */
export function RunbookMarkdown({ markdown }: { markdown: string }) {
  return (
    <div className="runbook-prose">
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          a: ({ href, children }) => (
            <a href={href} target="_blank" rel="noopener noreferrer">
              {children}
            </a>
          ),
        }}
      >
        {markdown}
      </ReactMarkdown>
    </div>
  );
}
