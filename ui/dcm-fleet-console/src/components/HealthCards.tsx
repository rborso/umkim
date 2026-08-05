import { Card, CardBody, CardTitle, Gallery, GalleryItem, Label } from '@patternfly/react-core';

export interface HealthEntry {
  label: string;
  ok: boolean;
  detail: string;
}

export function HealthCards({ entries }: { entries: HealthEntry[] }) {
  return (
    <Gallery hasGutter minWidths={{ default: '240px' }}>
      {entries.map((e) => (
        <GalleryItem key={e.label}>
          <Card isCompact>
            <CardTitle>{e.label}</CardTitle>
            <CardBody>
              <Label color={e.ok ? 'green' : 'red'}>{e.ok ? 'OK' : 'FAIL'}</Label>
              <div
                style={{
                  fontSize: '0.8rem',
                  marginTop: '0.35rem',
                  color: '#6a6e73',
                  wordBreak: 'break-word',
                  overflowWrap: 'anywhere',
                  maxHeight: '4.5rem',
                  overflow: 'auto',
                }}
              >
                {e.detail}
              </div>
            </CardBody>
          </Card>
        </GalleryItem>
      ))}
    </Gallery>
  );
}
