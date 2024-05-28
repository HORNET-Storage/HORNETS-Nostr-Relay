import { useEffect } from 'react';
import { useDispatch } from 'react-redux';
import { setPWASupported } from '@app/store/slices/pwaSlice';

export const usePWA = () => {
    const dispatch = useDispatch();

    useEffect(() => {
        const handler = (e: Event) => {
            e.preventDefault();
            // Assuming e is the BeforeInstallPromptEvent
            // Instead of storing the event, we just indicate support is available
            dispatch(setPWASupported(true));
        };

        window.addEventListener('beforeinstallprompt', handler);

        return () => {
            window.removeEventListener('beforeinstallprompt', handler);
            // Optionally, dispatch that PWA is no longer supported if the event is dismissed
            dispatch(setPWASupported(false));
        };
    }, [dispatch]);
};


// import { useEffect } from 'react';
// import { useDispatch } from 'react-redux';
// import { addDeferredPrompt } from '@app/store/slices/pwaSlice';

// export const usePWA = (): void => {
//   const dispatch = useDispatch();

//   useEffect(() => {
//     const handler = (e: Event) => {
//       e.preventDefault();
//       dispatch(addDeferredPrompt(e));
//     };

//     window.addEventListener('beforeinstallprompt', handler);
//   }, [dispatch]);
// };
