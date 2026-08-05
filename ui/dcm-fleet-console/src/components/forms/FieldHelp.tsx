import { Popover } from '@patternfly/react-core';
import { HelpIcon } from '@patternfly/react-icons';
import type { FieldHelpContent } from '../../help/fieldHelp';

/** PatternFly Form group label help button (see FormGroup labelIcon examples). */
const FORM_GROUP_LABEL_HELP_CLASS = 'pf-v5-c-form__group-label-help';

export type FieldHelpProps = FieldHelpContent & {
  fieldId?: string;
};

function ExampleBlock({ value }: { value: string }) {
  const multiline = value.includes('\n');
  if (multiline) {
    return (
      <pre
        style={{
          fontSize: '0.75rem',
          margin: '0.35rem 0 0',
          padding: '0.5rem',
          overflowX: 'auto',
          background: 'var(--pf-v5-global--BackgroundColor--200)',
        }}
      >
        {value}
      </pre>
    );
  }
  return (
    <code style={{ fontSize: '0.8rem' }}>{value}</code>
  );
}

export function FieldHelp({
  description,
  example,
  examples,
  whenToUse,
  commonMistakes,
  relatedFields,
  learnMoreUrl,
  fieldId,
}: FieldHelpProps) {
  const allExamples = [
    ...(example ? [example] : []),
    ...(examples ?? []),
  ];
  const testId = fieldId ? `field-help-${fieldId}` : undefined;

  const body = (
    <div style={{ maxWidth: '24rem' }}>
      <p style={{ margin: '0 0 0.5rem' }}>{description}</p>
      {whenToUse ? (
        <p style={{ margin: '0 0 0.5rem', fontSize: '0.85rem' }}>
          <strong>When to use:</strong> {whenToUse}
        </p>
      ) : null}
      {allExamples.length > 0 ? (
        <div>
          <strong style={{ fontSize: '0.8rem' }}>
            {allExamples.length === 1 ? 'Example' : 'Examples'}
          </strong>
          {allExamples.map((ex, i) => (
            <div key={i} style={{ marginTop: '0.25rem' }}>
              <ExampleBlock value={ex} />
            </div>
          ))}
        </div>
      ) : null}
      {commonMistakes && commonMistakes.length > 0 ? (
        <div style={{ marginTop: '0.5rem' }}>
          <strong style={{ fontSize: '0.8rem' }}>Common mistakes</strong>
          <ul style={{ margin: '0.25rem 0 0', paddingLeft: '1.1rem', fontSize: '0.8rem' }}>
            {commonMistakes.map((m) => (
              <li key={m}>{m}</li>
            ))}
          </ul>
        </div>
      ) : null}
      {relatedFields && relatedFields.length > 0 ? (
        <p style={{ margin: '0.5rem 0 0', fontSize: '0.8rem' }}>
          <strong>Related:</strong> {relatedFields.join(' · ')}
        </p>
      ) : null}
      {learnMoreUrl ? (
        <p style={{ margin: '0.5rem 0 0', fontSize: '0.8rem' }}>
          <a href={learnMoreUrl} target="_blank" rel="noreferrer">
            Learn more
          </a>
        </p>
      ) : null}
    </div>
  );

  return (
    <Popover
      aria-label="Field help"
      bodyContent={body}
      showClose={false}
      position="right"
    >
      <button
        type="button"
        aria-label="Show field help"
        onClick={(e) => e.preventDefault()}
        className={FORM_GROUP_LABEL_HELP_CLASS}
        data-testid={testId}
      >
        <HelpIcon />
      </button>
    </Popover>
  );
}
