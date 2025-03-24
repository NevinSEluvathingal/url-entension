from flask import Flask, request, jsonify
from transformers import pipeline

app = Flask(__name__)

# Load sentiment analysis model (1-5 score)
classifier = pipeline("sentiment-analysis", model="nlptown/bert-base-multilingual-uncased-sentiment")

@app.route('/analyze_sentiment', methods=['POST'])
def analyze_sentiment():
    try:
        data = request.json
        text = data.get("text", "")
        
        if not text:
            return jsonify({"error": "No text provided"}), 400

        # Get sentiment prediction
        result = classifier(text)
        score = int(result[0]["label"][0])  # Extracts 1-5 score from label

        return jsonify({"sentiment_score": score})
    
    except Exception as e:
        return jsonify({"error": str(e)}), 500

if __name__ == '__main__':
    app.run(debug=True, port=5000)

