#!/usr/bin/env python3
"""
Simple OCR Service for Price Extraction
Extracts only numeric values from YOLO-detected regions
"""

import os
import sys
import json
import base64
import re
import subprocess
import tempfile
from io import BytesIO
from PIL import Image
from flask import Flask, request, jsonify
import logging
import pytesseract

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

app = Flask(__name__)

class SimpleOCRExtractor:
    def __init__(self):
        # Tesseract configuration for number extraction
        self.tesseract_config = '--psm 7 --oem 3 -c tessedit_char_whitelist=0123456789., '
        
    def extract_numbers_from_image(self, image_data, bbox=None):
        """Extract only numeric values from image or cropped region"""
        try:
            # Convert bytes to PIL Image
            image = Image.open(BytesIO(image_data))
            
            # Crop to bbox if provided
            if bbox and len(bbox) == 4:
                x1, y1, x2, y2 = bbox
                # Ensure coordinates are within image bounds
                width, height = image.size
                x1 = max(0, int(x1))
                y1 = max(0, int(y1))
                x2 = min(width, int(x2))
                y2 = min(height, int(y2))
                
                # Crop the image
                image = image.crop((x1, y1, x2, y2))
                logger.info(f"Processing cropped region: {x1},{y1},{x2},{y2}")
            else:
                logger.info("Processing full image (no bbox provided)")
            
            # Convert to grayscale for better OCR
            if image.mode != 'L':
                image = image.convert('L')
            
            # Extract text using Tesseract
            text = pytesseract.image_to_string(image, config=self.tesseract_config)
            logger.info(f"Raw OCR text: '{text}'")
            
            # Extract the first numeric value from the raw text
            # Since we're only scanning the YOLO region, the raw text should be the price
            price = self.extract_first_number(text)
            
            if price is not None:
                logger.info(f"Extracted price: {price} from raw text: '{text}'")
                return {
                    'success': True,
                    'price': price,
                    'extracted_text': text,
                    'raw_text': text,  # Include full raw text for debugging
                    'confidence': 100,  # Simple confidence for numeric extraction
                    'strategy': 'raw_ocr'
                }
            else:
                logger.warning("No numeric value found in raw text")
                return {
                    'success': False,
                    'error': 'No numeric value found',
                    'extracted_text': text,
                    'raw_text': text  # Include full raw text for debugging
                }
                
        except Exception as e:
            logger.error(f"Error extracting numbers: {e}")
            return {
                'success': False,
                'error': str(e)
            }
    
    def extract_first_number(self, text):
        """Extract the first numeric value from text"""
        if not text:
            return None
        
        # Clean the text
        text = re.sub(r'\s+', ' ', text.strip())
        
        # Find the first numeric pattern (decimal or integer)
        # Try decimal format first (e.g., 4.90, 12.99)
        decimal_match = re.search(r'\d+\.\d+', text)
        if decimal_match:
            try:
                return float(decimal_match.group())
            except ValueError:
                pass
        
        # Try European decimal format (e.g., 4,90)
        european_match = re.search(r'\d+,\d+', text)
        if european_match:
            try:
                # Convert European format to standard
                number_str = european_match.group().replace(',', '.')
                return float(number_str)
            except ValueError:
                pass
        
        # Try integer format
        integer_match = re.search(r'\d+', text)
        if integer_match:
            try:
                return float(integer_match.group())
            except ValueError:
                pass
        
        return None

# Initialize the extractor
ocr_extractor = SimpleOCRExtractor()

@app.route('/health', methods=['GET'])
def health():
    """Health check endpoint"""
    return jsonify({
        'status': 'healthy',
        'service': 'simple_ocr',
        'version': '1.0.0'
    })

@app.route('/extract-price', methods=['POST'])
def extract_price():
    """Extract numeric price from image"""
    try:
        # Get image data from request
        if 'image' not in request.files and 'image_data' not in request.json:
            return jsonify({'error': 'No image provided'}), 400
        
        # Handle base64 encoded image
        if 'image_data' in request.json:
            image_data = base64.b64decode(request.json['image_data'])
        else:
            # Handle file upload
            image_file = request.files['image']
            image_data = image_file.read()
        
        # Get bbox parameter for region-based extraction
        bbox = None
        if 'bbox' in request.json:
            bbox = request.json['bbox']
            logger.info(f"Processing image with bbox: {bbox}")
        
        # Extract numeric values
        result = ocr_extractor.extract_numbers_from_image(image_data, bbox)
        
        return jsonify(result)
        
    except Exception as e:
        logger.error(f"Error processing image: {e}")
        return jsonify({
            'success': False,
            'error': str(e)
        }), 500

@app.route('/test', methods=['GET'])
def test():
    """Test endpoint"""
    tesseract_version = subprocess.run(['tesseract', '--version'], 
                                      capture_output=True, text=True).stdout.split('\n')[0]
    return jsonify({
        'message': 'Simple OCR service is running',
        'tesseract_version': tesseract_version,
        'features': [
            'Numeric value extraction only',
            'YOLO region cropping',
            'Multiple number format support',
            'Simple and fast processing'
        ]
    })

if __name__ == '__main__':
    logger.info("Starting Simple OCR Price Extraction Service v1.0...")
    tesseract_version = subprocess.run(['tesseract', '--version'], capture_output=True, text=True).stdout.split('\n')[0]
    logger.info(f"Tesseract version: {tesseract_version}")
    app.run(host='0.0.0.0', port=5000, debug=False)
