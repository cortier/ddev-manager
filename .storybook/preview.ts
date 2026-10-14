import type { Preview } from '@storybook/html-vite';

const preview: Preview = {
    parameters: {
        layout: 'centered',
        controls: { disable: true },
        options: { storySort: { order: ['Extension', ['Popup', 'Settings']] } },
        a11y: { test: 'todo' },
    },
};

export default preview;
