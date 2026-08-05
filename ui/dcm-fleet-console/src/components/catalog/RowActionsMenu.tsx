import { useState } from 'react';
import {
  Dropdown,
  DropdownItem,
  DropdownList,
  MenuToggle,
  Button,
} from '@patternfly/react-core';
import { EllipsisVIcon, PlayIcon } from '@patternfly/react-icons';

export type RowAction = {
  key: string;
  label: string;
  onClick: () => void;
  isDanger?: boolean;
};

type Props = {
  actions: RowAction[];
  /** Show a dedicated Deploy button outside the kebab (recommended for catalog rows). */
  deployAction?: RowAction;
};

export function RowActionsMenu({ actions, deployAction }: Props) {
  const [open, setOpen] = useState(false);

  const menuActions = deployAction
    ? actions.filter((a) => a.key !== deployAction.key)
    : actions;

  return (
    <div
      style={{
        display: 'flex',
        flexWrap: 'wrap',
        gap: '0.35rem',
        alignItems: 'center',
        minWidth: '11rem',
      }}
    >
      {deployAction ? (
        <Button
          size="sm"
          variant="primary"
          icon={<PlayIcon />}
          onClick={(e) => {
            e.stopPropagation();
            deployAction.onClick();
          }}
          data-testid={`row-deploy-${deployAction.key}`}
        >
          Deploy
        </Button>
      ) : null}
      <Dropdown
        isOpen={open}
        onOpenChange={setOpen}
        popperProps={{ position: 'right' }}
        toggle={(toggleRef) => (
          <MenuToggle
            ref={toggleRef}
            variant="plain"
            onClick={(e) => {
              e.stopPropagation();
              setOpen(!open);
            }}
            aria-label="Row actions"
            isExpanded={open}
          >
            <EllipsisVIcon />
          </MenuToggle>
        )}
      >
        <DropdownList>
          {menuActions.map((action) => (
            <DropdownItem
              key={action.key}
              onClick={(e) => {
                e.stopPropagation();
                setOpen(false);
                action.onClick();
              }}
              {...(action.isDanger ? { className: 'pf-m-danger' } : {})}
            >
              {action.label}
            </DropdownItem>
          ))}
          {menuActions.length === 0 ? (
            <DropdownItem isDisabled>No actions</DropdownItem>
          ) : null}
        </DropdownList>
      </Dropdown>
    </div>
  );
}
