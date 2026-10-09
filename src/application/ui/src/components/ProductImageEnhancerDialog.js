import React, { useState, useEffect, useRef } from 'react';
import {
    Dialog,
    DialogTitle,
    DialogContent,
    DialogActions,
    Button,
    TextField,
    RadioGroup,
    FormControlLabel,
    Radio,
    FormControl,
    FormLabel,
    Switch,
    CircularProgress,
    Box,
    Typography,
    Grid,
} from '@mui/material';
import { generateEnhancedImage } from '../services/genAiService';

const ProductImageEnhancerDialog = ({
    open,
    onClose,
    productId,
    productData,
    baseImageUrl,
}) => {
    const [backgroundType, setBackgroundType] = useState('beach'); // 'beach', 'field', 'color', 'custom'
    const [customBackground, setCustomBackground] = useState('');
    const [backgroundColor, setBackgroundColor] = useState('#ffffff'); // Default white
    const [addPerson, setAddPerson] = useState(false);
    const [personDescription, setPersonDescription] = useState('');
    const [style] = useState('photorealistic'); // Or make this selectable
    const [generatedImageBase64, setGeneratedImageBase64] = useState(null);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);
    const [baseImageBase64, setBaseImageBase64] = useState(null);

    const imgRef = useRef(null);

    useEffect(() => {
        if (open && baseImageUrl) {
            setError(null);
            setBaseImageBase64(null);
        } else if (!open) {
            setBaseImageBase64(null);
            setGeneratedImageBase64(null);
            setError(null);
            setLoading(false);
        }
    }, [open, baseImageUrl]);

    const convertImageToBase64 = () => {
        if (!imgRef.current || !imgRef.current.complete || !imgRef.current.naturalWidth) {
            setError("Image not fully loaded yet. Please try again.");
            return;
        }

        try {
            const canvas = document.createElement('canvas');
            const ctx = canvas.getContext('2d');

            canvas.width = imgRef.current.naturalWidth;
            canvas.height = imgRef.current.naturalHeight;

            ctx.drawImage(imgRef.current, 0, 0);

            const dataUrl = canvas.toDataURL('image/jpeg', 0.95);
            const base64String = dataUrl.split(',')[1];

            setBaseImageBase64(base64String);
            setError(null);

            return base64String;
        } catch (err) {
            console.error("Error converting image to base64:", err);
            setError("Failed to process image: " + err.message);
            return null;
        }
    };


    const handleGenerate = async () => {
        const base64Data = convertImageToBase64();
        if (!base64Data) {
            return;
        }
        setLoading(true);
        setError(null);
        setGeneratedImageBase64(null);

        let finalBackgroundPrompt = '';
        if (backgroundType === 'beach') {
            finalBackgroundPrompt = 'a sunny beach with ocean waves';
        } else if (backgroundType === 'field') {
            finalBackgroundPrompt = 'a lush green field under a blue sky';
        } else if (backgroundType === 'color') {
            finalBackgroundPrompt = `a solid ${backgroundColor} background`;
        } else { // custom
            finalBackgroundPrompt = customBackground;
        }

        const payload = {
            product_id: productId,
            product_data: productData,
            image_base64: base64Data,
            background_prompt: finalBackgroundPrompt,
            person_description: addPerson ? personDescription : null,
            style: style,
        };

        try {
            const result = await generateEnhancedImage(payload);

            if (result.error) {
                setError(result.error);
            } else if (result.generated_image_base64) {
                setGeneratedImageBase64(result.generated_image_base64);
            } else {
                setError("Unexpected response from server.");
            }
        } catch (err) {
            console.error('Error generating enhanced image:', err);
            setError(err.message || 'An unexpected error occurred.');
        } finally {
            setLoading(false);
        }
    };

    const handleClose = () => {
        // Reset state before closing if needed, or rely on useEffect cleanup
        onClose();
    };


    return (
        <Dialog open={open} onClose={handleClose} maxWidth="md" fullWidth>
            <DialogTitle>Enhance Product Image</DialogTitle>
            <DialogContent>
                <Grid container spacing={3}>
                    {/* Column 1: Original Image & Options */}
                    <Grid item xs={12} md={6}>
                        <Typography variant="h6" gutterBottom>Original Image</Typography>
                        {baseImageUrl && (
                            <Box mb={2} sx={{ textAlign: 'center' }}>
                                <img
                                    ref={imgRef}
                                    src={baseImageUrl}
                                    alt="Original Product"
                                    style={{ maxWidth: '100%', maxHeight: '300px', border: '1px solid #ccc' }}
                                    crossOrigin="anonymous"
                                />
                            </Box>
                        )}

                        <FormControl component="fieldset" margin="normal" fullWidth disabled={addPerson}>
                            <FormLabel component="legend">Background Options</FormLabel>
                            <RadioGroup
                                row
                                aria-label="background-type"
                                name="background-type"
                                value={backgroundType}
                                onChange={(e) => setBackgroundType(e.target.value)}
                            >
                                <FormControlLabel value="beach" control={<Radio />} label="Beach" />
                                <FormControlLabel value="field" control={<Radio />} label="Green Field" />
                                <FormControlLabel value="color" control={<Radio />} label="Solid Color" />
                                <FormControlLabel value="custom" control={<Radio />} label="Custom" />
                            </RadioGroup>
                        </FormControl>

                        {backgroundType === 'color' && !addPerson && (
                            <TextField
                                type="color"
                                label="Background Color"
                                value={backgroundColor}
                                onChange={(e) => setBackgroundColor(e.target.value)}
                                fullWidth
                                margin="dense"
                                disabled={addPerson}
                            />
                        )}
                        {backgroundType === 'custom' && !addPerson && (
                            <TextField
                                label="Custom Background Description"
                                value={customBackground}
                                onChange={(e) => setCustomBackground(e.target.value)}
                                fullWidth
                                margin="dense"
                                multiline
                                rows={2}
                                disabled={addPerson}
                            />
                        )}

                        <FormControlLabel
                            control={
                                <Switch
                                    checked={addPerson}
                                    onChange={(e) => setAddPerson(e.target.checked)}
                                    name="addPerson"
                                    color="primary"
                                />
                            }
                            label="Add Person Wearing Product"
                            sx={{ mt: 1, display: 'block' }}
                        />

                        {addPerson && (
                            <TextField
                                label="Describe the Person (e.g., 'woman smiling', 'man looking thoughtful')"
                                value={personDescription}
                                onChange={(e) => setPersonDescription(e.target.value)}
                                fullWidth
                                margin="dense"
                                multiline
                                rows={3}
                            />
                        )}
                    </Grid>

                    {/* Column 2: Generated Image */}
                    <Grid item xs={12} md={6}>
                        <Typography variant="h6" gutterBottom>Generated Image</Typography>
                        <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '300px', border: '1px dashed #ccc', mb: 2, position: 'relative' }}>
                            {loading && (
                                <CircularProgress sx={{ position: 'absolute' }} />
                            )}
                            {generatedImageBase64 && !loading && (
                                <img
                                    src={`data:image/png;base64,${generatedImageBase64}`} // Assuming PNG
                                    alt="Generated Product"
                                    style={{ maxWidth: '100%', maxHeight: '100%' }}
                                />
                            )}
                            {!generatedImageBase64 && !loading && (
                                <Typography variant="body2" color="textSecondary">
                                    Generated image will appear here.
                                </Typography>
                            )}
                        </Box>
                        {error && (
                            <Typography color="error" variant="body2">
                                Error: {error}
                            </Typography>
                        )}
                    </Grid>
                </Grid>
            </DialogContent>
            <DialogActions>
                <Button onClick={handleClose} color="secondary" disabled={loading}>
                    Cancel
                </Button>
                <Button
                    onClick={handleGenerate}
                    color="primary"
                    variant="contained"
                    disabled={loading || (addPerson && !personDescription) || (!addPerson && backgroundType === 'custom' && !customBackground)}
                >
                    {loading ? 'Generating...' : 'Generate Image'}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default ProductImageEnhancerDialog;
